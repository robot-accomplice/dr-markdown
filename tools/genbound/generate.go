package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"sort"
	"strings"
)

// Param is one argument of a bound method, with its type rendered as written.
type Param struct {
	Name string
	Type string
}

// Method is one method bound to the frontend.
type Method struct {
	Name    string
	Params  []Param
	Results []string
	Guarded bool
}

// ReturnsError reports whether the last result is an error, which is the only
// result the dispatcher treats specially: it rejects the frontend's promise
// instead of resolving it.
func (m Method) ReturnsError() bool {
	return len(m.Results) > 0 && m.Results[len(m.Results)-1] == "error"
}

// ReturnsValue reports whether there is a value to resolve with, as opposed to
// only an error or nothing at all.
func (m Method) ReturnsValue() bool {
	if m.ReturnsError() {
		return len(m.Results) > 1
	}
	return len(m.Results) > 0
}

// Imports maps a package qualifier as it appears in a type ("preferences") to
// the path it was imported from. Aliased imports keep their alias, because that
// is what the type expressions in this file will say.
type Imports map[string]string

// ParseImports reads app.go's import block. The generated dispatcher mentions
// qualified types verbatim — preferences.Preferences, imageassets.ImportedImage
// — so it needs the same imports under the same names or it will not compile.
func ParseImports(src string) (Imports, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "app.go", src, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("parse imports: %w", err)
	}

	out := Imports{}
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		name := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		out[name] = path
	}
	return out, nil
}

// qualifiersIn returns the package qualifiers the generated file will actually
// NAME.
//
// Parameters only. Results are received with `:=` so their types are inferred
// and never written down — importing for them produces an unused-import error
// in the generated file, which is a compile failure a long way from its cause.
func qualifiersIn(methods []Method) map[string]bool {
	seen := map[string]bool{}
	note := func(typ string) {
		// Strip whatever wraps the named type: []T, map[K]V, *T.
		for _, part := range strings.FieldsFunc(typ, func(r rune) bool {
			return r == '[' || r == ']' || r == '*' || r == ' '
		}) {
			if i := strings.Index(part, "."); i > 0 {
				seen[part[:i]] = true
			}
		}
	}
	for _, m := range methods {
		for _, p := range m.Params {
			note(p.Type)
		}
	}
	return seen
}

// ParseApp returns the exported methods on *App, in declaration order.
//
// go/ast rather than a regular expression, on purpose. Every regex detector
// this project has written was wrong at least once: one matched 17 of 18
// methods because a method was written on a single line, and reported the
// method it never looked at as clean.
func ParseApp(src string) ([]Method, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "app.go", src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse app.go: %w", err)
	}

	var out []Method
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || !fn.Name.IsExported() {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := star.X.(*ast.Ident); !ok || ident.Name != "App" {
			continue
		}

		m := Method{Name: fn.Name.Name, Guarded: firstStatementGuards(fn, fn.Name.Name)}
		for _, field := range fn.Type.Params.List {
			typ := render(fset, field.Type)
			for _, name := range field.Names {
				m.Params = append(m.Params, Param{Name: name.Name, Type: typ})
			}
		}
		if fn.Type.Results != nil {
			for _, field := range fn.Type.Results.List {
				m.Results = append(m.Results, render(fset, field.Type))
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// render prints a type expression exactly as it was written, so `[]OpenDocument`
// and `map[string]string` survive into the generated file unchanged.
func render(fset *token.FileSet, expr ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, expr); err != nil {
		return "any"
	}
	return b.String()
}

// firstStatementGuards reports whether the method's FIRST statement is
// `defer a.reportPanic("<name>")`. Position matters: a guard placed after other
// statements does not cover them.
func firstStatementGuards(fn *ast.FuncDecl, name string) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	def, ok := fn.Body.List[0].(*ast.DeferStmt)
	if !ok {
		return false
	}
	sel, ok := def.Call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "reportPanic" || len(def.Call.Args) != 1 {
		return false
	}
	lit, ok := def.Call.Args[0].(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == `"`+name+`"`
}

// EmitDispatch renders the typed dispatch table.
//
// Generated rather than reflected on purpose: a reflective dispatcher resolves
// methods by name string at runtime, which is the thing the host boundary set
// out to replace. Here a renamed or re-signatured method is a COMPILE error in
// a tracked file, not a call that silently stops working.
func EmitDispatch(methods []Method, imports Imports) string {
	var b strings.Builder
	b.WriteString("// Code generated by tools/genbound. DO NOT EDIT.\n")
	b.WriteString("// Regenerate with: go run ./tools/genbound\n\n")
	b.WriteString("//go:build darwin\n\n")
	b.WriteString("package main\n\n")
	b.WriteString("import (\n\t\"encoding/json\"\n")
	// Only the qualifiers the signatures actually mention. Emitting every import
	// from app.go would produce unused-import errors in the generated file.
	var qualified []string
	for q := range qualifiersIn(methods) {
		if path, ok := imports[q]; ok {
			qualified = append(qualified, path)
		}
	}
	sort.Strings(qualified)
	if len(qualified) > 0 {
		b.WriteString("\n")
		for _, path := range qualified {
			name := path[strings.LastIndex(path, "/")+1:]
			for alias, p := range imports {
				if p == path && alias != name {
					name = alias
				}
			}
			if name == path[strings.LastIndex(path, "/")+1:] {
				fmt.Fprintf(&b, "\t%q\n", path)
			} else {
				fmt.Fprintf(&b, "\t%s %q\n", name, path)
			}
		}
	}
	b.WriteString(")\n\n")
	b.WriteString("// dispatchBound calls a bound method with typed arguments decoded from the\n")
	b.WriteString("// frontend's JSON. `handled` is false when no such method exists, so the\n")
	b.WriteString("// caller can tell \"unknown method\" from \"method returned nothing\".\n")
	b.WriteString("func dispatchBound(app *App, name string, args []json.RawMessage) (result any, err error, handled bool) {\n")
	b.WriteString("\tswitch name {\n")

	for _, m := range methods {
		fmt.Fprintf(&b, "\tcase %q:\n", m.Name)

		names := make([]string, len(m.Params))
		for i, p := range m.Params {
			fmt.Fprintf(&b, "\t\tvar a%d %s\n", i, p.Type)
			names[i] = fmt.Sprintf("a%d", i)
		}
		if len(m.Params) > 0 {
			fmt.Fprintf(&b, "\t\tif err := decodeArgs(args, %s); err != nil {\n",
				"&"+strings.Join(names, ", &"))
			b.WriteString("\t\t\treturn nil, err, true\n\t\t}\n")
		}

		call := fmt.Sprintf("app.%s(%s)", m.Name, strings.Join(names, ", "))
		switch {
		case m.ReturnsValue() && m.ReturnsError():
			fmt.Fprintf(&b, "\t\tv, err := %s\n", call)
			b.WriteString("\t\treturn v, err, true\n")
		case m.ReturnsError():
			fmt.Fprintf(&b, "\t\treturn nil, %s, true\n", call)
		case m.ReturnsValue():
			fmt.Fprintf(&b, "\t\treturn %s, nil, true\n", call)
		default:
			fmt.Fprintf(&b, "\t\t%s\n", call)
			b.WriteString("\t\treturn nil, nil, true\n")
		}
	}

	b.WriteString("\t}\n\treturn nil, nil, false\n}\n")
	return b.String()
}
