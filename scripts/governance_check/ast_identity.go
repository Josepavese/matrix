package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

// astRuleIdentityComparison is the ast_rule value that turns a budget into a
// shape check over the parsed syntax tree instead of a literal count.
const astRuleIdentityComparison = "identity_comparison"

// identityComparison is one equality test between a NAMED value and a string
// literal, where the name denotes an identity — which agent, provider, program
// or model is being talked to — rather than a classifier — what kind of thing
// this is.
type identityComparison struct {
	RelativePath string
	Line         int
	Name         string
	Literal      string
	Expression   string
}

// pair is the reviewed_pairs key: the name and the literal it is compared to.
// Reviewing the pair, not the name, is what keeps the check blind to a literal
// nobody reviewed: `name == "fork"` may be a protocol capability, while
// `name == "brandnewagent"` on the same variable is the branch this budget
// exists to catch.
func (c identityComparison) pair() string {
	return c.Name + "=" + c.Literal
}

func (c identityComparison) String() string {
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

// checkIdentityComparisons walks the budget's roots and returns every equality
// test that compares a value whose role is an identity against a string literal
// that no reviewer has cleared.
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
		ast.Inspect(file, func(node ast.Node) bool {
			literal, other, ok := comparedLiteral(node)
			if !ok {
				return true
			}
			name, ok := namedValue(other)
			if !ok {
				return true
			}
			if !identityRole(identity, name) {
				return true
			}
			finding := identityComparison{
				RelativePath: rel,
				Line:         fset.Position(node.Pos()).Line,
				Name:         name,
				Literal:      literal,
			}
			if _, cleared := reviewed[finding.pair()]; cleared {
				return true
			}
			finding.Expression = renderExpression(fset, other)
			findings = append(findings, finding)
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].RelativePath != findings[j].RelativePath {
			return findings[i].RelativePath < findings[j].RelativePath
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}
