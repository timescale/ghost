package writer

import (
	"io"
	"sync"

	"github.com/timescale/ghost/internal/serve/api"
)

type Outputs []Output

type Output struct {
	api.Output

	// For endpoint destination.
	PipeReaderChan      chan *io.PipeReader
	closePipeReaderChan func()
}

func NewOutputs(apiOutputs api.Outputs) Outputs {
	outputs := make(Outputs, len(apiOutputs))
	for i, output := range apiOutputs {
		pipeReaderChan, closeReaderChan := newPipeReaderChan(output)
		outputs[i] = Output{
			Output:              output,
			PipeReaderChan:      pipeReaderChan,
			closePipeReaderChan: closeReaderChan,
		}
	}
	return outputs
}

func newPipeReaderChan(o api.Output) (chan *io.PipeReader, func()) {
	if o.Destination == api.OutputDestinationEndpoint {
		pipeReaderChan := make(chan *io.PipeReader)
		closePipeReaderChan := sync.OnceFunc(func() {
			close(pipeReaderChan)
		})
		return pipeReaderChan, closePipeReaderChan
	}
	return nil, nil
}

func (o Outputs) EndpointOutput(format api.OutputFormat) (Output, bool) {
	for _, output := range o {
		if output.Destination == api.OutputDestinationEndpoint && output.Format == format {
			return output, true
		}
	}
	return Output{}, false
}

func (o Outputs) Close() {
	for _, output := range o {
		output.Close()
	}
}

func (o *Output) ContentType() string {
	switch o.Format {
	case api.OutputFormatArrowStream:
		return "application/vnd.apache.arrow.stream"
	case api.OutputFormatArrowFile:
		return "application/vnd.apache.arrow.file"
	case api.OutputFormatParquet:
		return "application/vnd.apache.parquet"
	case api.OutputFormatCSV:
		return "text/csv"
	default:
		return ""
	}
}

func (o *Output) ContentEncoding() string {
	// Parquet format only gzips the internal row groups, not the entire
	// output stream, so don't set Content-Encoding to gzip.
	if o.Compression == api.OutputCompressionGzip &&
		o.Format != api.OutputFormatParquet {
		return "gzip"
	}
	return ""
}

func (o *Output) Close() {
	if o.Destination == api.OutputDestinationEndpoint {
		o.closePipeReaderChan()
	}
}
