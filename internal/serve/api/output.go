package api

// OutputFormat represents the supported formats that run results can be
// written to.
type OutputFormat string

const (
	OutputFormatArrowStream OutputFormat = "arrowStream"
	OutputFormatArrowFile   OutputFormat = "arrowFile"
	OutputFormatParquet     OutputFormat = "parquet"
	OutputFormatCSV         OutputFormat = "csv"
	OutputFormatTSV         OutputFormat = "tsv"
	OutputFormatJSON        OutputFormat = "json"
)

// OutputDestination represents the supported destinations that run results can
// be written to.
type OutputDestination string

const (
	OutputDestinationEndpoint OutputDestination = "endpoint"
	OutputDestinationFile     OutputDestination = "file"
)

// OutputCompression represents the supported types of compression that can be
// applied to run results before they are written to a destination.
type OutputCompression string

const (
	OutputCompressionNone   OutputCompression = ""
	OutputCompressionBrotli OutputCompression = "brotli"
	OutputCompressionGzip   OutputCompression = "gzip"
	OutputCompressionLZ4    OutputCompression = "lz4"
	OutputCompressionSnappy OutputCompression = "snappy"
	OutputCompressionZstd   OutputCompression = "zstd"
)

// Outputs represents a slice of [Output].
type Outputs []Output

// Output represents a request to write run results to a specific destination
// (e.g. the results endpoint or the local filesystem) in a specific format
// (e.g. Arrow IPC Streaming/File Format, Parquet, CSV, or TSV). It also
// specifies whether gzip compression should be used, and whether the special
// __popsql_row_num__ field should be included in the results.
type Output struct {
	Format      OutputFormat      `json:"format"`
	Destination OutputDestination `json:"destination"`
	Compression OutputCompression `json:"compression,omitempty"`
	RowNum      bool              `json:"rowNum,omitempty"`
	Limit       int64             `json:"limit,omitempty"`

	// For CSV/TSV formats.
	NullValue string `json:"nullValue,omitempty"`

	// For file destination.
	Path string `json:"path,omitempty"`
}
