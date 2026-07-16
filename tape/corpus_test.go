package tape_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GH-Jaider/foley/tape"
	"github.com/GH-Jaider/foley/tape/internal/vhsgrammar/lexer"
	"github.com/GH-Jaider/foley/tape/internal/vhsgrammar/parser"
)

// TestVHSCorpusConformance parses every tape of the vendored upstream
// corpus with the vendored grammar — the whole point of ADR-008: if VHS
// ships it as an example, foley parses it identically. Each tape parses
// from its own directory (Source resolves CWD-relative, exactly like
// VHS's evaluator). Failures are only legal on the explicit list below.
func TestVHSCorpusConformance(t *testing.T) {
	// Upstream's intentionally-broken fixtures that fail at PARSE time.
	// errors/dimensions.tape and errors/require.tape parse clean — their
	// failures are evaluator-time upstream, so they belong to the
	// executor's error tests, not here.
	expectedBad := map[string]string{
		"errors/parser.tape": "upstream parser-error fixture",
	}

	root := filepath.Join("internal", "vhsgrammar", "examples")
	var tapes []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".tape") {
			tapes = append(tapes, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The corpus shrinking silently would gut this suite's meaning.
	if len(tapes) < 100 {
		t.Fatalf("corpus has %d tapes, expected >= 100 — did vendoring lose files?", len(tapes))
	}

	for _, tp := range tapes {
		rel, err := filepath.Rel(root, tp)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(tp) //nolint:gosec // vendored corpus path
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(filepath.Dir(tp))
			p := parser.New(lexer.New(string(data)))
			cmds := p.Parse()
			errs := p.Errors()
			if reason, bad := expectedBad[rel]; bad {
				if len(errs) == 0 {
					t.Fatalf("expected to fail (%s) but parsed clean", reason)
				}
				return
			}
			if len(errs) > 0 {
				t.Fatalf("parse errors:\n%v", errs)
			}
			if len(cmds) == 0 {
				t.Fatal("tape parsed to zero commands")
			}
			// The TYPED layer must also swallow the whole corpus: every
			// grammar-valid tape converts (tapes without their own
			// Output are legal upstream — they exist to be Sourced).
			typed, err := tape.Parse(string(data))
			if err != nil && !strings.Contains(err.Error(), "no Output declared") {
				t.Fatalf("typed Parse: %v", err)
			}
			_ = typed
		})
	}
}
