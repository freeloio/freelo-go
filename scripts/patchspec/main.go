// Command patchspec preprocesses the downloaded OpenAPI document before
// oapi-codegen runs, flattening scalar `oneOf` unions in *parameter* schemas
// into a plain `type: string`.
//
// Why this exists: Freelo's spec describes polymorphic path parameters with a
// oneOf, e.g. note_id accepts either the numeric id or the note uuid:
//
//	NoteIdParam:
//	  name: note_id
//	  in: path
//	  schema:
//	    oneOf:
//	      - type: integer
//	      - type: string
//	        format: uuid
//
// oapi-codegen v2 turns that into a union struct whose accessors reference
// generated-but-never-emitted member types (`N0`, `N1`) — which additionally
// collide with the unprefixed `N0`/`N1` enum constants it emits elsewhere, so
// the package does not compile ("N0 (constant) is not a type"). Even if it did
// compile, a union struct cannot be serialized into a URL path segment by
// runtime.StyleParamWithOptions.
//
// Path, query, header and cookie parameters are strings on the wire, so
// collapsing an all-scalar union to `type: string` is lossless: callers can
// pass "12345" or a uuid in the same argument.
//
// This is a spec preprocessor (rather than a post-generation patch) because the
// fix belongs to the input document: one schema edit replaces having to rewrite
// a union struct plus six accessor methods in the generated Go.
//
// Only parameter schemas are touched — request/response body unions keep their
// generated union types. Run via `make gen`. Idempotent, and a no-op once the
// upstream spec stops emitting scalar parameter unions.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const specFile = "spec/freelo-api.yaml"

// Scalar branch forms we are willing to collapse: `- type: integer` and the
// keys that may follow inside the same branch (format, description, …).
var (
	inKeyRe        = regexp.MustCompile(`^in:\s*(path|query|header|cookie)\s*$`)
	branchStartRe  = regexp.MustCompile(`^-\s+type:\s*(integer|number|string|boolean)\s*$`)
	branchDetailRe = regexp.MustCompile(`^(format|description|example|pattern|minimum|maximum|minLength|maxLength):`)
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "patchspec:", err)
		os.Exit(1)
	}
}

func run() error {
	src, err := os.ReadFile(specFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", specFile, err)
	}

	lines := strings.Split(string(src), "\n")
	flattened := 0

	// Walk backwards so replacing a range never shifts indices we still need.
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "schema:" {
			continue
		}
		indent := indentOf(lines[i])
		if !isParameterSchema(lines, i, indent) {
			continue
		}
		end, ok := scalarUnionEnd(lines, i, indent)
		if !ok {
			continue
		}
		replacement := strings.Repeat(" ", indent+2) + "type: string"
		lines = append(lines[:i+1], append([]string{replacement}, lines[end:]...)...)
		flattened++
	}

	if flattened == 0 {
		fmt.Println("patchspec: no scalar parameter unions found (nothing to do)")
		return nil
	}

	if err := os.WriteFile(specFile, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return fmt.Errorf("write %s: %w", specFile, err)
	}
	fmt.Printf("patchspec: flattened %d scalar parameter union(s) to type: string in %s\n", flattened, specFile)
	return nil
}

// isParameterSchema reports whether the `schema:` key at lines[i] belongs to an
// OpenAPI Parameter Object, i.e. whether it has a sibling `in: path|query|…`
// key at the same indentation inside the same mapping.
func isParameterSchema(lines []string, i, indent int) bool {
	for _, j := range siblingRange(lines, i, indent) {
		if inKeyRe.MatchString(strings.TrimSpace(lines[j])) {
			return true
		}
	}
	return false
}

// siblingRange returns the indices of the keys sitting at exactly `indent`
// within the mapping that contains lines[i] (the mapping is bounded by the
// nearest lines above and below with a smaller indentation).
func siblingRange(lines []string, i, indent int) []int {
	var out []int
	for j := i - 1; j >= 0; j-- {
		if blank(lines[j]) {
			continue
		}
		if in := indentOf(lines[j]); in < indent {
			break
		} else if in == indent {
			out = append(out, j)
		}
	}
	for j := i + 1; j < len(lines); j++ {
		if blank(lines[j]) {
			continue
		}
		if in := indentOf(lines[j]); in < indent {
			break
		} else if in == indent {
			out = append(out, j)
		}
	}
	return out
}

// scalarUnionEnd checks that the `schema:` at lines[i] holds nothing but a
// oneOf of scalar types, and returns the index of the first line after that
// schema body. ok is false for any other schema shape ($ref, object/array
// branches, sibling keys next to the oneOf, …).
func scalarUnionEnd(lines []string, i, indent int) (end int, ok bool) {
	j := i + 1
	for j < len(lines) && blank(lines[j]) {
		j++
	}
	if j >= len(lines) || indentOf(lines[j]) <= indent || strings.TrimSpace(lines[j]) != "oneOf:" {
		return 0, false
	}
	oneOfIdx := j
	oneOfIndent := indentOf(lines[j])

	branches := 0
	for j++; j < len(lines); j++ {
		if blank(lines[j]) {
			continue
		}
		if indentOf(lines[j]) <= oneOfIndent {
			break // end of the oneOf list
		}
		switch text := strings.TrimSpace(lines[j]); {
		case branchStartRe.MatchString(text):
			branches++
		case branchDetailRe.MatchString(text):
			// extra keys on the current scalar branch — fine
		default:
			return 0, false // $ref, nested object/array, enum list, …
		}
	}
	if branches < 2 {
		return 0, false
	}
	// Keep blank lines that separate the schema from whatever follows.
	for j > oneOfIdx+1 && blank(lines[j-1]) {
		j--
	}

	// The schema must contain only the oneOf; a sibling key (e.g. `nullable:`)
	// would be silently dropped by the rewrite.
	if len(siblingRange(lines, oneOfIdx, oneOfIndent)) > 0 {
		return 0, false
	}
	return j, true
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func blank(line string) bool {
	return strings.TrimSpace(line) == ""
}
