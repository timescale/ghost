package function

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// typeResolver resolves pg_type OIDs into typeInfo — unwrapping arrays and
// domains and collecting enum labels — from catalog rows preloaded in bulk.
// One resolver lives for one introspection pass: introspect preloads every
// OID its functions reference, and resolve then works entirely in memory.
type typeResolver struct {
	pool  *pgxpool.Pool
	types map[int64]typeRow
}

func newTypeResolver(pool *pgxpool.Pool) *typeResolver {
	return &typeResolver{
		pool:  pool,
		types: map[int64]typeRow{},
	}
}

// preload fetches the catalog rows for the given type OIDs in batches. Types
// can reference further types (an array its element, a domain its base), so
// referenced OIDs that weren't already loaded are fetched in a follow-up
// batch; in practice that means one query, or two when arrays or domains are
// involved. After preload, resolve never touches the database.
func (r *typeResolver) preload(ctx context.Context, oids []int64) error {
	pending := r.missing(oids)
	for len(pending) > 0 {
		rows, err := r.pool.Query(ctx, typesQuery, pending)
		if err != nil {
			return fmt.Errorf("failed to load type info: %w", err)
		}
		fetched, err := pgx.CollectRows(rows, pgx.RowToStructByName[typeRow])
		if err != nil {
			return fmt.Errorf("failed to load type info: %w", err)
		}

		var referenced []int64
		for _, t := range fetched {
			r.types[t.OID] = t
			referenced = append(referenced, t.Elem, t.Base)
		}
		pending = r.missing(referenced)
	}
	return nil
}

// missing returns the deduplicated OIDs that have not been loaded yet.
func (r *typeResolver) missing(oids []int64) []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, oid := range oids {
		if oid == 0 || seen[oid] {
			continue
		}
		seen[oid] = true
		if _, ok := r.types[oid]; !ok {
			out = append(out, oid)
		}
	}
	return out
}

// resolve returns the typeInfo for a preloaded type OID. Arrays resolve to
// their element type with IsArray set, and domains resolve to their base
// type, so the resulting Name is always a name the JSON Schema and scan-type
// mappings understand (unknown names deliberately degrade to strings there).
// Pseudo-types (anyelement, internal, ...) are rejected: values of these
// types can't cross the tool boundary.
func (r *typeResolver) resolve(oid int64) (typeInfo, error) {
	t, ok := r.types[oid]
	if !ok {
		return typeInfo{}, fmt.Errorf("type %d was not loaded from the catalog", oid)
	}

	switch {
	case t.Category == "A" && t.Elem != 0:
		// Array: resolve the element type. PostgreSQL does not track array
		// dimensionality in the type system, so every array is exposed as a
		// single-level JSON array.
		elemInfo, err := r.resolve(t.Elem)
		if err != nil {
			return typeInfo{}, err
		}
		if elemInfo.IsArray {
			return typeInfo{}, fmt.Errorf("unsupported nested array type %q", t.Name)
		}
		return typeInfo{
			Name:     elemInfo.Name,
			IsArray:  true,
			EnumVals: elemInfo.EnumVals,
		}, nil
	case t.TypeType == "d":
		// Domain: expose it as its base type.
		return r.resolve(t.Base)
	case t.TypeType == "e":
		return typeInfo{
			Name:     t.Name,
			EnumVals: t.EnumLabels,
		}, nil
	case t.TypeType == "p":
		return typeInfo{}, fmt.Errorf("unsupported pseudo-type %q", t.Name)
	default:
		// Base, composite, range, and multirange types all render and scan
		// through their canonical text form; names the schema/scan mappings
		// don't recognize degrade to plain strings.
		return typeInfo{Name: t.Name}, nil
	}
}

// typeRow is the catalog row for one type, with enum labels attached.
type typeRow struct {
	OID        int64    `db:"oid"`
	Name       string   `db:"name"`
	TypeType   string   `db:"type_type"`
	Category   string   `db:"category"`
	Elem       int64    `db:"elem"`
	Base       int64    `db:"base"`
	EnumLabels []string `db:"enum_labels"` // nil for non-enums
}

// typesQuery loads the catalog facts needed to classify a batch of types.
const typesQuery = `
SELECT
    t.oid::int8 AS oid,
    pg_catalog.format_type(t.oid, NULL) AS name,
    t.typtype::text AS type_type,
    t.typcategory::text AS category,
    t.typelem::int8 AS elem,
    t.typbasetype::int8 AS base,
    (
        SELECT array_agg(enumlabel ORDER BY enumsortorder)
        FROM pg_catalog.pg_enum
        WHERE enumtypid = t.oid
    ) AS enum_labels
FROM pg_catalog.pg_type t
WHERE t.oid::int8 = ANY($1)`
