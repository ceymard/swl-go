package json

import (
	"context"
	"os"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Source reads JSON/JSON5 and emits one or more collections.
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.File == "" {
		return nil, errs.New("json source requires a file path or inline JSON")
	}

	inline := fileIsInlineJSON(opts.File)
	var data []byte
	if inline {
		data = []byte(opts.File)
	} else {
		enc := "utf-8"
		if opts.Encoding != nil {
			enc = *opts.Encoding
		}
		if enc != "utf-8" && enc != "" {
			return nil, errs.New("json source: only utf-8 encoding is supported")
		}
		b, err := os.ReadFile(opts.File)
		if err != nil {
			return nil, errs.Wrap(err, "read json file", "path", opts.File)
		}
		data = b
	}

	return streamFromBytes(ctx, data, opts.File, opts.Collection, inline)
}
