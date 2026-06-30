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
	Options any      // typed opts from optparse
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
		name, explicitSource := handler.SplitSourcePrefix(seg.tokens[0])
		wantSink := seg.afterColon && !explicitSource
		id, kind, err := resolveHandler(name, wantSink)
		if err != nil {
			return Pipeline{}, err
		}
		// Classify stage kind: transforms (flatten) vs file/db sinks.
		if explicitSource {
			kind = StageSource // +handler is always a source, even after ::
		} else if seg.afterColon && handler.IsTransformOnly(id) {
			kind = StageTransform
		} else if seg.afterColon && i == len(raw)-1 {
			kind = StageSink // last segment after :: is always the terminal sink
		} else if seg.afterColon && kind == StageSource {
			kind = StageSink // e.g. out.json resolved as json-src id but used as sink
		}

		target, tail := stageTarget(seg.tokens, id, kind)
		opts, err := handler.ParseOptions(id, target, tail)
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

// stageTarget extracts the path/URI (target) and flag tail from a segment.
//
//	flatten -o x     → target="", tail=[-o,x]
//	data.json        → target=data.json, tail=[]
//	json data.json   → target=data.json, tail=[]
func stageTarget(tokens []string, id string, kind StageKind) (target string, tail []string) {
	if len(tokens) == 0 {
		return "", nil
	}
	if handler.IsTransformOnly(id) {
		return "", tokens[1:]
	}
	wantSink := kind == StageSink
	first, _ := handler.SplitSourcePrefix(tokens[0])
	if _, _, ok := handler.ResolveAlias(first, wantSink); ok {
		if len(tokens) > 1 {
			return tokens[1], tokens[2:]
		}
		return "", tokens[1:]
	}
	return tokens[0], tokens[1:]
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
			afterColon = false // legacy: chain another source (prefer :: +src)
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
// Order: alias name, file extension, URI protocol prefix, inline JSON.
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
	// Inline JSON literal as source (swl2 passes through unmatched tokens).
	if !wantSink && len(target) > 0 && (target[0] == '[' || target[0] == '{') {
		return "json-src", StageSource, nil
	}
	return "", 0, errs.New("cannot resolve handler for " + target)
}
