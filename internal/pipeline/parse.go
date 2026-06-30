// Package pipeline parses the swl CLI argv into ordered stages.
//
// Example: swl users.json ++ orders.csv :: flatten :: app.db
//
//	segments: [users.json] [orders.csv] [flatten] [app.db]
//	kinds:    source       source       transform sink
package pipeline

import (
	"path/filepath"
	"strings"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/stage"
	"github.com/ceymard/swl-go/handler"
)

type StageKind = stage.Kind

const (
	StageSource    = stage.Source
	StageTransform = stage.Transform
	StageSink      = stage.Sink
)

// Stage is one element of the pipeline after parsing.
type Stage struct {
	Kind    StageKind
	ID      string   // handler registry id, e.g. "json-src", "flatten"
	Tokens  []string // raw argv segment (for error messages)
	Options any      // typed opts from participle parser
}

// Pipeline is the full parsed command.
type Pipeline struct {
	Stages  []Stage
	Verbose int
}

// Parse turns argv (after global kong flags) into a Pipeline.
func Parse(argv []string, verbose int) (Pipeline, error) {
	raw, err := splitSegments(argv)
	if err != nil {
		return Pipeline{}, err
	}

	var stages []Stage
	for i, seg := range raw {
		if len(seg.tokens) == 0 {
			return Pipeline{}, errs.New("empty pipeline segment")
		}
		wantSink := seg.afterColon
		id, kind, err := resolveHandler(seg.tokens[0], wantSink)
		if err != nil {
			return Pipeline{}, err
		}
		// Classify stage kind: transforms (flatten) vs file/db sinks.
		if seg.afterColon && handler.IsTransformOnly(id) {
			kind = StageTransform
		} else if seg.afterColon && i == len(raw)-1 {
			kind = StageSink // last segment after :: is always the terminal sink
		} else if seg.afterColon && kind == StageSource {
			kind = StageSink // e.g. out.json resolved as json-src id but used as sink
		}

		opts, err := handler.ParseOptions(id, seg.tokens[1:])
		if err != nil {
			return Pipeline{}, errs.Wrap(err, "parse options", "handler", id)
		}

		stages = append(stages, Stage{
			Kind:    kind,
			ID:      id,
			Tokens:  seg.tokens,
			Options: opts,
		})
	}

	return Pipeline{Stages: stages, Verbose: verbose}, nil
}

// segment is one argv chunk between :: or ++ separators.
type segment struct {
	tokens     []string
	afterColon bool // true if this segment follows a :: (sink side of pipeline)
}

// splitSegments splits on :: and ++ while remembering which side of :: each part is on.
func splitSegments(argv []string) ([]segment, error) {
	var segments []segment
	var current []string
	var afterColon bool

	flush := func() {
		if len(current) == 0 {
			return
		}
		segments = append(segments, segment{
			tokens:     append([]string(nil), current...),
			afterColon: afterColon,
		})
		current = nil
	}

	for _, tok := range argv {
		switch tok {
		case "::":
			flush()
			afterColon = true // following segments are sink-side until next ++
		case "++":
			flush()
			afterColon = false // chained source
		default:
			current = append(current, tok)
		}
	}
	flush()

	if len(segments) == 0 {
		return nil, errs.New("empty pipeline")
	}
	return segments, nil
}

// resolveHandler maps the first token of a segment to a handler id.
// Order: alias name, file extension, URI protocol prefix.
func resolveHandler(target string, wantSink bool) (id string, kind StageKind, err error) {
	if h, k, ok := handler.ResolveAlias(target, wantSink); ok {
		return h, k, nil
	}
	ext := strings.ToLower(filepath.Ext(target))
	if h, k, ok := handler.ResolveExtension(ext, wantSink); ok {
		return h, k, nil
	}
	if idx := strings.Index(target, "://"); idx > 0 {
		proto := target[:idx+3]
		if h, k, ok := handler.ResolveProtocol(proto, wantSink); ok {
			return h, k, nil
		}
	}
	return "", 0, errs.New("cannot resolve handler for " + target)
}
