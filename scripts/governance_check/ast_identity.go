package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// astRuleIdentityComparison is the ast_rule value that turns a budget into a
// shape check over the parsed syntax tree instead of a literal count.
const astRuleIdentityComparison = "identity_comparison"

// identityComparison is one equality test between a NAMED value and a string
// literal — or a constant whose value is a string literal — where the name
// denotes an identity — which agent, provider, program or model is being talked
// to — rather than a classifier — what kind of thing this is.
type identityComparison struct {
	RelativePath string
	Line         int
	Name         string
	Literal      string
	// Constant is set when the compared value is a package constant rather than
	// a literal written in place. The constant is what a reader has to open to
	// see the branch, which is exactly why it hid from the literal budgets.
	Constant string
	// DenominatesAgent records whether that constant's value names a known agent.
	// It does not decide the finding — the shape does, as it does for a literal —
	// it decides what the finding says, so a reviewer sees at a glance whether the
	// branch is on which agent or on some other closed vocabulary.
	DenominatesAgent bool
	Expression       string
}

// pair is the reviewed_pairs key: the name and the literal it is compared to.
// Reviewing the pair, not the name, is what keeps the check blind to a literal
// nobody reviewed: `name == "fork"` may be a protocol capability, while
// `name == "brandnewagent"` on the same variable is the branch this budget
// exists to catch.
//
// A constant comparison carries the constant's name instead of its value, in the
// form `name=const:ConstName`. Keying it by the name means renaming the constant
// re-opens the review, and the `const:` marker means a cleared literal can never
// silently clear a constant.
func (c identityComparison) pair() string {
	if c.Constant != "" {
		return c.Name + "=const:" + c.Constant
	}
	return c.Name + "=" + c.Literal
}

func (c identityComparison) String() string {
	if c.Constant != "" {
		reading := "whose value names no agent in the vocabulary"
		if c.DenominatesAgent {
			reading = "which names an agent"
		}
		return fmt.Sprintf("%s:%d compares %s against const %s (%q), %s", c.RelativePath, c.Line, c.Expression, c.Constant, c.Literal, reading)
	}
	return fmt.Sprintf("%s:%d compares %s against %q", c.RelativePath, c.Line, c.Expression, c.Literal)
}

// identityLiteralIgnored reports whether a literal can never be an identity.
// An empty literal is the "is this set?" guard, which is the most common
// legitimate shape in the tree; a one-character or whitespace-bearing literal
// is a separator, a path fragment or a message, not the name of a program.
func identityLiteralIgnored(literal string) bool {
	trimmed := strings.TrimSpace(literal)
	if len(trimmed) < 2 {
		return true
	}
	return strings.ContainsAny(trimmed, " \t\r\n")
}

// namedValue reports the name a compared expression carries. A map lookup keyed
// by a string literal names its field as clearly as a struct field does:
// state.Context["provider"] carries the same role as state.Provider, so the key
// is the name. A call, a dereference or an arithmetic expression carries no
// name, and this check does not guess.
func namedValue(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name, true
	case *ast.SelectorExpr:
		return value.Sel.Name, true
	case *ast.IndexExpr:
		key, ok := value.Index.(*ast.BasicLit)
		if !ok || key.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(key.Value)
		if err != nil {
			return "", false
		}
		return "[" + strings.TrimSpace(text) + "]", true
	case *ast.ParenExpr:
		return namedValue(value.X)
	}
	return "", false
}

// comparedLiteral extracts the string literal of an equality test whose other
// operand is a named value. It is deliberately narrow: only `==` and `!=`
// against a quoted string, because that is the shape a name-based branch takes.
func comparedLiteral(node ast.Node) (literal string, other ast.Expr, ok bool) {
	binary, ok := node.(*ast.BinaryExpr)
	if !ok || (binary.Op != token.EQL && binary.Op != token.NEQ) {
		return "", nil, false
	}
	var lit *ast.BasicLit
	switch {
	case isStringLiteral(binary.X):
		lit, _ = binary.X.(*ast.BasicLit)
		other = binary.Y
	case isStringLiteral(binary.Y):
		lit, _ = binary.Y.(*ast.BasicLit)
		other = binary.X
	default:
		return "", nil, false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil || identityLiteralIgnored(value) {
		return "", nil, false
	}
	return value, other, true
}

func isStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// identityRole reports whether a name carries an identity role. A map key is
// spelled [provider] so the failure message shows what the code says, but the
// role it carries is provider: the vocabulary lists a role once and covers both
// spellings, so a reviewer cannot forget the bracketed twin of a name they just
// added.
func identityRole(identity map[string]struct{}, name string) bool {
	if _, ok := identity[name]; ok {
		return true
	}
	inner, found := strings.CutPrefix(name, "[")
	if !found {
		return false
	}
	inner, found = strings.CutSuffix(inner, "]")
	if !found {
		return false
	}
	_, ok := identity[inner]
	return ok
}

func renderExpression(fset *token.FileSet, expr ast.Expr) string {
	var buf strings.Builder
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return "?"
	}
	text := buf.String()
	if len(text) > 60 {
		text = text[:60]
	}
	return text
}

// constKey identifies a package-level constant by the directory that declares it
// and its name. Package-level only: a constant declared inside a function is not
// visible outside it, and treating two same-named locals as one value would
// invent a branch the tree does not contain.
type constKey struct {
	dir  string
	name string
}

// pendingIdentityComparison is a comparison whose constant side cannot be judged
// until every file has been read: the constant may be declared in a file the
// walk has not reached yet, or in another package.
type pendingIdentityComparison struct {
	rel          string
	pos          token.Pos
	line         int
	name         string
	expression   string
	constDisplay string
	constKey     constKey
}

// agentDenominates reports whether a value denominates an agent: it carries one
// of the vocabulary's names as a WORD of its own, case-insensitively. "codex" and
// "codex-acp" denominate the codex agent; "mimosa" does not denominate MiMo,
// because there the name is part of a longer word. The whole-word rule keeps the
// label honest: a constant that merely contains the letters names no agent.
//
// This labels a finding, it does not gate one. Gating on the value was measured
// and rejected: a scratch `agentID == newAgentConst` carrying an agent name
// nobody had enumerated passed the gate, while gating on the shape costs exactly
// one review in this tree — a protocol constant, not an agent.
func agentDenominates(values []string, value string) bool {
	lowered := strings.ToLower(strings.TrimSpace(value))
	if lowered == "" {
		return false
	}
	for _, name := range values {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		for at := 0; at+len(name) <= len(lowered); {
			i := strings.Index(lowered[at:], name)
			if i < 0 {
				break
			}
			start := at + i
			if !isWordByte(lowered, start-1) && !isWordByte(lowered, start+len(name)) {
				return true
			}
			at = start + 1
		}
	}
	return false
}

func isWordByte(value string, i int) bool {
	if i < 0 || i >= len(value) {
		return false
	}
	c := value[i]
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_'
}

// collectPackageConsts binds the package-level string constants a file declares.
// A spec that binds several names to several values is read position by
// position, and a position that is not a plain string is skipped rather than
// guessed.
func collectPackageConsts(file *ast.File, dir string, into map[constKey]string) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Values) != len(value.Names) {
				continue
			}
			for i, name := range value.Names {
				lit, ok := value.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				into[constKey{dir: dir, name: name.Name}] = text
			}
		}
	}
}

// modulePath reads the module path from go.mod so an import path can be mapped
// back to the directory a constant lives in. A root without go.mod still works
// for same-package constants; cross-package resolution needs the module.
func modulePath(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, "module "); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// importPathFor resolves a qualifier to the import path it names. A qualifier
// that is not an import (a variable of a struct type, say) resolves to nothing,
// so `state.AgentName` is never mistaken for a package selector.
func importPathFor(file *ast.File, qualifier string) (string, bool) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := ""
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "_" || name == "." {
			continue
		}
		if name == "" {
			parts := strings.Split(path, "/")
			name = parts[len(parts)-1]
		}
		if name == qualifier {
			return path, true
		}
	}
	return "", false
}

// constReference resolves one side of a comparison to a package constant. It
// reports the constant as a reader would write it — `agentCodex` or
// `agentidentity.CanonicalCodexAgentID` — and where its value must be looked up.
func constReference(file *ast.File, dir, module string, expr ast.Expr) (string, constKey, bool) {
	switch value := expr.(type) {
	case *ast.ParenExpr:
		return constReference(file, dir, module, value.X)
	case *ast.Ident:
		return value.Name, constKey{dir: dir, name: value.Name}, true
	case *ast.SelectorExpr:
		qualifier, ok := value.X.(*ast.Ident)
		if !ok {
			return "", constKey{}, false
		}
		path, ok := importPathFor(file, qualifier.Name)
		if !ok || module == "" {
			return "", constKey{}, false
		}
		relative, found := strings.CutPrefix(path, module+"/")
		if !found {
			return "", constKey{}, false
		}
		return qualifier.Name + "." + value.Sel.Name, constKey{dir: relative, name: value.Sel.Name}, true
	}
	return "", constKey{}, false
}

// constComparisonCandidates records the comparisons whose constant side has to be
// resolved once every file has been read. Both operand orders are considered, so
// `CanonicalCodexAgentID == agentID` is caught like its mirror image.
func constComparisonCandidates(fset *token.FileSet, file *ast.File, rel, dir, module string, node ast.Node) []pendingIdentityComparison {
	binary, ok := node.(*ast.BinaryExpr)
	if !ok || (binary.Op != token.EQL && binary.Op != token.NEQ) {
		return nil
	}
	var out []pendingIdentityComparison
	consider := func(constSide, other ast.Expr) {
		display, key, ok := constReference(file, dir, module, constSide)
		if !ok {
			return
		}
		name, ok := namedValue(other)
		if !ok {
			return
		}
		out = append(out, pendingIdentityComparison{
			rel: rel, pos: node.Pos(), line: fset.Position(node.Pos()).Line, name: name,
			expression: renderExpression(fset, other), constDisplay: display, constKey: key,
		})
	}
	consider(binary.X, binary.Y)
	consider(binary.Y, binary.X)
	return out
}

// checkIdentityComparisons walks the budget's roots and returns every equality
// test that compares a value whose role is an identity against a string literal
// — or a package constant standing in for one — that no reviewer has cleared.
// The constant is caught by its SHAPE, like the literal: the value being one
// indirection away is the whole reason it used to pass unseen.
//
// It complements the textual budgets instead of replacing them: a literal name
// anywhere in production is still caught by the name budgets, while this check
// catches the SHAPE on a variable the name budgets never enumerated. A file that
// does not parse is a failure, not a skip: leaving it out would exempt exactly
// the code nobody can read.
func checkIdentityComparisons(root string, budget patternBudget) ([]identityComparison, error) {
	identity := make(map[string]struct{}, len(budget.IdentityNames))
	for _, name := range budget.IdentityNames {
		identity[strings.TrimSpace(name)] = struct{}{}
	}
	reviewed := make(map[string]struct{}, len(budget.ReviewedPairs))
	for _, pair := range budget.ReviewedPairs {
		reviewed[strings.TrimSpace(pair)] = struct{}{}
	}
	module := modulePath(root)
	consts := make(map[constKey]string)
	var pending []pendingIdentityComparison
	var findings []identityComparison

	err := walkBudgetFiles(root, budget, func(rel, full string) error {
		if !strings.HasSuffix(rel, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, full, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("%s: %w", rel, parseErr)
		}
		dir := filepath.Dir(rel)
		collectPackageConsts(file, dir, consts)
		ast.Inspect(file, func(node ast.Node) bool {
			literal, other, ok := comparedLiteral(node)
			if ok {
				if name, named := namedValue(other); named && identityRole(identity, name) {
					finding := identityComparison{
						RelativePath: rel,
						Line:         fset.Position(node.Pos()).Line,
						Name:         name,
						Literal:      literal,
					}
					if _, cleared := reviewed[finding.pair()]; !cleared {
						finding.Expression = renderExpression(fset, other)
						findings = append(findings, finding)
					}
				}
			}
			pending = append(pending, constComparisonCandidates(fset, file, rel, dir, module, node)...)
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// A constant is judged here, once every declaration is known: the value is
	// what makes the comparison a branch on which agent, and a name is not a
	// value. One comparison yields one finding even when both operands resolve.
	seen := make(map[string]struct{}, len(pending))
	for _, candidate := range pending {
		value, found := consts[candidate.constKey]
		if !found || identityLiteralIgnored(value) {
			continue
		}
		if !identityRole(identity, candidate.name) {
			continue
		}
		key := fmt.Sprintf("%s:%d", candidate.rel, candidate.pos)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		finding := identityComparison{
			RelativePath:     candidate.rel,
			Line:             candidate.line,
			Name:             candidate.name,
			Literal:          value,
			Constant:         candidate.constDisplay,
			DenominatesAgent: agentDenominates(budget.AgentNameValues, value),
			Expression:       candidate.expression,
		}
		if _, cleared := reviewed[finding.pair()]; cleared {
			continue
		}
		findings = append(findings, finding)
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].RelativePath != findings[j].RelativePath {
			return findings[i].RelativePath < findings[j].RelativePath
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}
