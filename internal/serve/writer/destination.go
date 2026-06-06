package writer

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/timescale/ghost/internal/serve/api"
)

// newDestinationWriter returns an [io.WriteCloser] configured to write to the
// destination represented by the provided [Output] - i.e. either to the
// response of a separate endpoint call or a file on the local filesystem.
func newDestinationWriter(ctx context.Context, output Output) (io.WriteCloser, error) {
	switch output.Destination {
	case api.OutputDestinationEndpoint:
		return newEndpointWriter(ctx, output)
	case api.OutputDestinationFile:
		return newFileWriter(output)
	default:
		return nil, fmt.Errorf("unexpected destination type: %s", output.Destination)
	}
}

func newEndpointWriter(ctx context.Context, output Output) (io.WriteCloser, error) {
	defer output.Close()

	pipeReader, pipeWriter := io.Pipe()
	select {
	case output.PipeReaderChan <- pipeReader:
		return pipeWriter, nil
	case <-ctx.Done():
		// If the query was canceled before a request was made to retrieve the
		// results from the results endpoint, stop waiting (the results will
		// not be available to be retrieved).
		return nil, ctx.Err()
	}
}

func newFileWriter(output Output) (io.WriteCloser, error) {
	// Create the file for writing, truncate it if it already exists
	return os.OpenFile(output.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
}
