package main

import (
	"fmt"
	"go/types"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/ettle/strcase"
)

const (
	templateImports = `// Code generated by mocktail; DO NOT EDIT.

package {{ .Name }}

{{ if .Imports }}import (
{{- range $index, $import := .Imports }}
	{{ if $import }}"{{ $import }}"{{ else }}{{end}}
{{- end}}
){{end}}
`

	templateMockBase = `
// {{ .InterfaceName | ToGoCamel }}Mock mock of {{ .InterfaceName }}.
type {{ .InterfaceName | ToGoCamel }}Mock struct { mock.Mock }

// new{{ .InterfaceName | ToGoPascal }}Mock creates a new {{ .InterfaceName | ToGoCamel }}Mock.
func new{{ .InterfaceName | ToGoPascal }}Mock(tb testing.TB) *{{ .InterfaceName | ToGoCamel }}Mock {
	tb.Helper()

	m := &{{ .InterfaceName | ToGoCamel }}Mock{}
	m.Mock.Test(tb)

	tb.Cleanup(func() { m.AssertExpectations(tb) })

	return m
}
`

	templateCallBase = `
type {{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call struct{
	*mock.Call
	Parent *{{ .InterfaceName | ToGoCamel }}Mock
}


func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) Panic(msg string) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.Panic(msg)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) Once() *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.Once()
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) Twice() *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.Twice()
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) Times(i int) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.Times(i)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) WaitUntil(w <-chan time.Time) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.WaitUntil(w)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) After(d time.Duration) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.After(d)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) Run(fn func(args mock.Arguments)) *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.Run(fn)
	return _c
}

func (_c *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call) Maybe() *{{ .InterfaceName | ToGoCamel }}{{ .MethodName }}Call {
	_c.Call = _c.Call.Maybe()
	return _c
}

`
)

// Syrup generates method mocks and mock.Call wrapper.
type Syrup struct {
	PackageName   string
	InterfaceName string
	Method        *types.Func
	Signature     *types.Signature
}

// MockMethod generates method mocks.
func (s Syrup) MockMethod(writer io.Writer) error {
	err := s.mockedMethod(writer)
	if err != nil {
		return err
	}

	err = s.methodOn(writer)
	if err != nil {
		return err
	}

	return s.methodOnRaw(writer)
}

func (s Syrup) mockedMethod(writer io.Writer) error {
	w := &Writer{writer: writer}

	w.Printf("func (_m *%sMock) %s(", strcase.ToGoCamel(s.InterfaceName), s.Method.Name())

	params := s.Signature.Params()

	var argNames []string
	for i := 0; i < params.Len(); i++ {
		param := params.At(i)

		if param.Type().String() == contextType {
			w.Print("_")
		} else {
			name := getParamName(param, i)
			w.Print(name)
			argNames = append(argNames, name)
		}

		w.Print(" " + s.getTypeName(param.Type()))

		if i+1 < params.Len() {
			w.Print(", ")
		}
	}

	w.Print(") ")

	results := s.Signature.Results()

	if results.Len() > 1 {
		w.Print("(")
	}

	for i := 0; i < results.Len(); i++ {
		w.Print(s.getTypeName(results.At(i).Type()))
		if i+1 < results.Len() {
			w.Print(", ")
		}
	}

	if results.Len() > 1 {
		w.Print(")")
	}

	w.Println(" {")

	w.Print("\t")
	if results.Len() > 0 {
		w.Print("_ret := ")
	}
	w.Printf("_m.Called(%s)\n", strings.Join(argNames, ", "))

	s.writeReturnsFnCaller(w, argNames, params, results)

	for i := 0; i < results.Len(); i++ {
		if i == 0 {
			w.Println()
		}

		rType := results.At(i).Type()

		w.Printf("\t%s", getResultName(results.At(i), i))

		switch rType.String() {
		case "string", "int", "bool", "error":
			w.Printf("\t := _ret.%s(%d)\n", strcase.ToPascal(rType.String()), i)
		default:
			name := s.getTypeName(rType)
			w.Printf(", _ := _ret.Get(%d).(%s)\n", i, name)
		}
	}

	for i := 0; i < results.Len(); i++ {
		if i == 0 {
			w.Println()
			w.Print("\treturn ")
		}

		w.Print(getResultName(results.At(i), i))

		if i+1 < results.Len() {
			w.Print(", ")
		} else {
			w.Println()
		}
	}

	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) writeReturnsFnCaller(w *Writer, argNames []string, params, results *types.Tuple) {
	if len(argNames) > 0 && results.Len() > 0 {
		w.Println()
		w.Printf("\tif _rf, ok := _ret.Get(0).(%s); ok {\n", s.createFuncSignature(params, results))
		w.Printf("\t\treturn _rf(%s)\n", strings.Join(argNames, ", "))
		w.Println("\t}")
	}
}

func (s Syrup) createFuncSignature(params, results *types.Tuple) string {
	fnSign := "func ("
	for i := 0; i < params.Len(); i++ {
		param := params.At(i)
		if param.Type().String() == contextType {
			continue
		}

		fnSign += s.getTypeName(param.Type())

		if i+1 < params.Len() {
			fnSign += ", "
		}
	}
	fnSign += ") "

	fnSign += "("
	for i := 0; i < results.Len(); i++ {
		rType := results.At(i).Type()
		fnSign += s.getTypeName(rType)
		if i+1 < results.Len() {
			fnSign += ", "
		}
	}
	fnSign += ")"

	return fnSign
}

func (s Syrup) methodOn(writer io.Writer) error {
	w := &Writer{writer: writer}

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	w.Printf("func (_m *%sMock) On%s(", structBaseName, s.Method.Name())

	params := s.Signature.Params()

	var argNames []string
	for i := 0; i < params.Len(); i++ {
		param := params.At(i)

		if param.Type().String() == contextType {
			continue
		}

		name := getParamName(param, i)

		w.Print(name)
		argNames = append(argNames, name)

		w.Print(" " + s.getTypeName(param.Type()))

		if i+1 < params.Len() {
			w.Print(", ")
		}
	}

	w.Printf(") *%s%sCall {\n", structBaseName, s.Method.Name())

	w.Printf(`	return &%s%sCall{Call: _m.Mock.On("%s", %s), Parent: _m}`,
		structBaseName, s.Method.Name(), s.Method.Name(), strings.Join(argNames, ", "))

	w.Println()
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) methodOnRaw(writer io.Writer) error {
	w := &Writer{writer: writer}

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	w.Printf("func (_m *%sMock) On%sRaw(", structBaseName, s.Method.Name())

	params := s.Signature.Params()

	var argNames []string
	for i := 0; i < params.Len(); i++ {
		param := params.At(i)

		if param.Type().String() == contextType {
			continue
		}

		name := getParamName(param, i)

		w.Print(name)
		argNames = append(argNames, name)

		w.Print(" interface{}")

		if i+1 < params.Len() {
			w.Print(", ")
		}
	}

	w.Printf(") *%s%sCall {\n", structBaseName, s.Method.Name())

	w.Printf(`	return &%s%sCall{Call: _m.Mock.On("%s", %s), Parent: _m}`,
		structBaseName, s.Method.Name(), s.Method.Name(), strings.Join(argNames, ", "))

	w.Println()
	w.Println("}")
	w.Println()

	return w.Err()
}

// Call generates mock.Call wrapper.
func (s Syrup) Call(writer io.Writer, methods []*types.Func) error {
	err := s.callBase(writer)
	if err != nil {
		return err
	}

	err = s.typedReturns(writer)
	if err != nil {
		return err
	}

	err = s.returnsFn(writer)
	if err != nil {
		return err
	}

	err = s.callMethodsOn(writer, methods)
	if err != nil {
		return err
	}

	return s.callMethodOnRaw(writer, methods)
}

func (s Syrup) callBase(writer io.Writer) error {
	base := template.New("templateCallBase").Funcs(template.FuncMap{
		"ToGoCamel":  strcase.ToGoCamel,
		"ToGoPascal": strcase.ToGoPascal,
	})

	tmpl, err := base.Parse(templateCallBase)
	if err != nil {
		return err
	}

	data := map[string]string{
		"InterfaceName": s.InterfaceName,
		"MethodName":    s.Method.Name(),
	}

	return tmpl.Execute(writer, data)
}

func (s Syrup) typedReturns(writer io.Writer) error {
	w := &Writer{writer: writer}

	results := s.Signature.Results()
	if results.Len() <= 0 {
		return nil
	}

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	w.Printf("func (_c *%s%sCall) TypedReturns(", structBaseName, s.Method.Name())

	var returnNames string
	for i := 0; i < results.Len(); i++ {
		rName := string(rune(int('a') + i))

		w.Printf("%s %s", rName, s.getTypeName(results.At(i).Type()))
		returnNames += rName

		if i+1 < results.Len() {
			w.Print(", ")
			returnNames += ", "
		}
	}

	w.Printf(") *%s%sCall {\n", structBaseName, s.Method.Name())
	w.Printf("\t_c.Call = _c.Return(%s)\n", returnNames)
	w.Println("\treturn _c")
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) returnsFn(writer io.Writer) error {
	w := &Writer{writer: writer}

	results := s.Signature.Results()
	if results.Len() < 1 {
		return nil
	}

	params := s.Signature.Params()
	if params.Len() < 1 {
		return nil
	}

	structBaseName := strcase.ToGoCamel(s.InterfaceName)

	w.Printf("func (_c *%[1]s%[2]sCall) ReturnsFn(fn %[3]s) *%[1]s%[2]sCall {\n",
		structBaseName, s.Method.Name(), s.createFuncSignature(params, results))
	w.Println("\t_c.Call = _c.Return(fn)")
	w.Println("\treturn _c")
	w.Println("}")
	w.Println()

	return w.Err()
}

func (s Syrup) callMethodsOn(writer io.Writer, methods []*types.Func) error {
	w := &Writer{writer: writer}

	callType := fmt.Sprintf("%s%sCall", strcase.ToGoCamel(s.InterfaceName), s.Method.Name())

	for _, method := range methods {
		sign := method.Type().(*types.Signature)

		w.Printf("func (_c *%s) On%s(", callType, method.Name())

		params := sign.Params()

		var argNames []string
		for i := 0; i < params.Len(); i++ {
			param := params.At(i)

			if param.Type().String() == contextType {
				continue
			}

			name := getParamName(param, i)

			w.Print(name)
			argNames = append(argNames, name)

			w.Print(" " + s.getTypeName(param.Type()))

			if i+1 < params.Len() {
				w.Print(", ")
			}
		}

		w.Printf(") *%s%sCall {\n", strcase.ToGoCamel(s.InterfaceName), method.Name())

		w.Printf("\treturn _c.Parent.On%s(%s)\n", method.Name(), strings.Join(argNames, ", "))
		w.Println("}")
		w.Println()
	}

	return w.Err()
}

func (s Syrup) callMethodOnRaw(writer io.Writer, methods []*types.Func) error {
	w := &Writer{writer: writer}

	callType := fmt.Sprintf("%s%sCall", strcase.ToGoCamel(s.InterfaceName), s.Method.Name())

	for _, method := range methods {
		sign := method.Type().(*types.Signature)

		w.Printf("func (_c *%s) On%sRaw(", callType, method.Name())

		params := sign.Params()

		var argNames []string
		for i := 0; i < params.Len(); i++ {
			param := params.At(i)

			if param.Type().String() == contextType {
				continue
			}

			name := getParamName(param, i)

			w.Print(name)
			argNames = append(argNames, name)

			w.Print(" interface{}")

			if i+1 < params.Len() {
				w.Print(", ")
			}
		}

		w.Printf(") *%s%sCall {\n", strcase.ToGoCamel(s.InterfaceName), method.Name())

		w.Printf("\treturn _c.Parent.On%sRaw(%s)\n", method.Name(), strings.Join(argNames, ", "))
		w.Println("}")
		w.Println()
	}

	return w.Err()
}

func (s Syrup) getTypeName(t types.Type) string {
	switch v := t.(type) {
	case *types.Basic:
		return v.Name()

	case *types.Slice:
		return "[]" + s.getTypeName(v.Elem())

	case *types.Map:
		return "map[" + s.getTypeName(v.Key()) + "]" + s.getTypeName(v.Elem())

	case *types.Named:
		name := v.String()

		i := strings.LastIndex(v.String(), "/")
		if i > -1 {
			name = name[i+1:]
		}

		if v.Obj() != nil && v.Obj().Pkg() != nil && v.Obj().Pkg().Name() == s.PackageName {
			return name[len(s.PackageName)+1:]
		}

		return name

	case *types.Pointer:
		return "*" + s.getTypeName(v.Elem())

	case *types.Interface:
		return v.String()

	default:
		panic(fmt.Sprintf("OOPS %[1]T %[1]s", t))
	}
}

func writeImports(writer io.Writer, pkg string, descPkg PackageDesc) error {
	base := template.New("templateImports")

	tmpl, err := base.Parse(templateImports)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"Name":    pkg,
		"Imports": quickGoImports(descPkg),
	}
	return tmpl.Execute(writer, data)
}

func writeMockBase(writer io.Writer, interfaceName string) error {
	base := template.New("templateMockBase").Funcs(template.FuncMap{
		"ToGoCamel":  strcase.ToGoCamel,
		"ToGoPascal": strcase.ToGoPascal,
	})

	tmpl, err := base.Parse(templateMockBase)
	if err != nil {
		return err
	}

	return tmpl.Execute(writer, map[string]string{"InterfaceName": interfaceName})
}

func quickGoImports(descPkg PackageDesc) []string {
	imports := []string{
		"testing",                          // require by test
		"time",                             // require by `WaitUntil(w <-chan time.Time)`
		"",                                 // to separate std imports than the others
		"github.com/stretchr/testify/mock", // require by mock
	}

	for imp := range descPkg.Imports {
		imports = append(imports, imp)
	}

	sort.Slice(imports, func(i, j int) bool {
		if imports[i] == "" {
			return strings.Contains(imports[j], ".")
		}
		if imports[j] == "" {
			return !strings.Contains(imports[i], ".")
		}

		if strings.Contains(imports[i], ".") && !strings.Contains(imports[j], ".") {
			return false
		}
		if !strings.Contains(imports[i], ".") && strings.Contains(imports[j], ".") {
			return true
		}

		return imports[i] < imports[j]
	})

	return imports
}

func getParamName(tVar *types.Var, i int) string {
	if tVar.Name() == "" {
		return fmt.Sprintf("%sParam", string(rune('a'+i)))
	}
	return tVar.Name()
}

func getResultName(tVar *types.Var, i int) string {
	if tVar.Name() == "" {
		return fmt.Sprintf("_r%s%d", string(rune('a'+i)), i)
	}
	return tVar.Name()
}

// Writer is a wrapper around Print+ functions.
type Writer struct {
	writer io.Writer
	err    error
}

// Err returns error from the other methods.
func (w *Writer) Err() error {
	return w.err
}

// Print formats using the default formats for its operands and writes to standard output.
func (w *Writer) Print(a ...interface{}) {
	if w.err != nil {
		return
	}

	_, w.err = fmt.Fprint(w.writer, a...)
}

// Printf formats according to a format specifier and writes to standard output.
func (w *Writer) Printf(pattern string, a ...interface{}) {
	if w.err != nil {
		return
	}

	_, w.err = fmt.Fprintf(w.writer, pattern, a...)
}

// Println formats using the default formats for its operands and writes to standard output.
func (w *Writer) Println(a ...interface{}) {
	if w.err != nil {
		return
	}

	_, w.err = fmt.Fprintln(w.writer, a...)
}
