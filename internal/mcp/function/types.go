package function

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// typeResolver resolves pg_type OIDs into TypeInfo, unwrapping arrays and
// domains and collecting enum labels. Results are cached per resolver, which
// lives for one introspection pass.
type typeResolver struct {
	pool  *pgxpool.Pool
	cache map[int64]TypeInfo
}

func newTypeResolver(pool *pgxpool.Pool) *typeResolver {
	return &typeResolver{
		pool:  pool,
		cache: make(map[int64]TypeInfo),
	}
}

// typeQuery loads the catalog facts needed to classify one type.
const typeQuery = `
SELECT
    pg_catalog.format_type(t.oid, NULL) AS name,
    t.typtype::text AS type_type,
    t.typcategory::text AS category,
    t.typelem::int8 AS elem,
    t.typbasetype::int8 AS base
FROM pg_catalog.pg_type t
WHERE t.oid = $1`

const enumLabelsQuery = `
SELECT enumlabel
FROM pg_catalog.pg_enum
WHERE enumtypid = $1
ORDER BY enumsortorder`

// resolve returns the TypeInfo for a type OID. Arrays resolve to their
// element type with IsArray set, and domains resolve to their base type, so
// the resulting Name is always a name the JSON Schema and scan-type mappings
// understand (unknown names deliberately degrade to strings there). Pseudo-
// types (anyelement, internal, ...) are rejected: values of these types
// can't cross the tool boundary.
func (r *typeResolver) resolve(ctx context.Context, oid int64) (TypeInfo, error) {
	if info, ok := r.cache[oid]; ok {
		return info, nil
	}

	var (
		name     string
		typeType string
		category string
		elem     int64
		base     int64
	)
	if err := r.pool.QueryRow(ctx, typeQuery, oid).Scan(&name, &typeType, &category, &elem, &base); err != nil {
		return TypeInfo{}, fmt.Errorf("failed to resolve type %d: %w", oid, err)
	}

	var info TypeInfo
	switch {
	case category == "A" && elem != 0:
		// Array: resolve the element type. PostgreSQL does not track array
		// dimensionality in the type system, so every array is exposed as a
		// single-level JSON array.
		elemInfo, err := r.resolve(ctx, elem)
		if err != nil {
			return TypeInfo{}, err
		}
		if elemInfo.IsArray {
			return TypeInfo{}, fmt.Errorf("unsupported nested array type %q", name)
		}
		info = TypeInfo{
			Name:     elemInfo.Name,
			IsArray:  true,
			EnumVals: elemInfo.EnumVals,
		}
	case typeType == "d":
		// Domain: expose it as its base type.
		return r.resolve(ctx, base)
	case typeType == "e":
		rows, err := r.pool.Query(ctx, enumLabelsQuery, oid)
		if err != nil {
			return TypeInfo{}, fmt.Errorf("failed to load enum labels for %q: %w", name, err)
		}
		labels, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return TypeInfo{}, fmt.Errorf("failed to load enum labels for %q: %w", name, err)
		}
		info = TypeInfo{
			Name:     name,
			EnumVals: labels,
		}
	case typeType == "p":
		return TypeInfo{}, fmt.Errorf("unsupported pseudo-type %q", name)
	default:
		// Base, composite, range, and multirange types all render and scan
		// through their canonical text form; names the schema/scan mappings
		// don't recognize degrade to plain strings.
		info = TypeInfo{Name: name}
	}

	r.cache[oid] = info
	return info, nil
}
