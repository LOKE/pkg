// Command lokerpcgen extracts RPC documentation and string enums from Go source.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const defaultOutput = "lokerpc_metadata.gen.go"

type stringFlags []string

func (f *stringFlags) String() string { return strings.Join(*f, ",") }

func (f *stringFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

type packageInfo struct {
	Dir        string
	ImportPath string
	Name       string
	GoFiles    []string
}

type typeDeclaration struct {
	doc  string
	spec *ast.TypeSpec
}

type sourcePackage struct {
	info   packageInfo
	types  map[string]typeDeclaration
	consts map[string][]enumValue
}

type enumValue struct {
	name  string
	value string
}

type constantDeclaration struct {
	name       string
	typeName   string
	expression ast.Expr
}

type generatedType struct {
	description string
	fields      map[string]string
	enum        []enumValue
}

type generatedService struct {
	description string
	methods     map[string]string
}

type generatedPackage struct {
	info     packageInfo
	types    map[string]generatedType
	services map[string]generatedService
}

func main() {
	var services stringFlags
	var rootTypes stringFlags
	output := flag.String("output", defaultOutput, "generated Go filename")
	flag.Var(&services, "service", "service interface to document (repeatable)")
	flag.Var(&rootTypes, "type", "root type to document (repeatable)")
	flag.Parse()

	if len(services) == 0 && len(rootTypes) == 0 {
		fail("at least one -service interface or -type is required")
	}

	info, err := loadPackageInfo()
	if err != nil {
		fail("load package: %v", err)
	}
	source, err := parsePackage(info, *output)
	if err != nil {
		fail("parse package: %v", err)
	}
	metadata, err := buildMetadata(source, services, rootTypes)
	if err != nil {
		fail("build metadata: %v", err)
	}
	generated, err := render(metadata)
	if err != nil {
		fail("render metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(info.Dir, *output), generated, 0o644); err != nil {
		fail("write metadata: %v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "lokerpcgen: "+format+"\n", args...)
	os.Exit(1)
}

func loadPackageInfo() (packageInfo, error) {
	command := exec.Command("go", "list", "-json", ".")
	command.Stderr = os.Stderr
	output, err := command.Output()
	if err != nil {
		return packageInfo{}, err
	}
	var info packageInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return packageInfo{}, err
	}
	return info, nil
}

func parsePackage(info packageInfo, output string) (sourcePackage, error) {
	fset := token.NewFileSet()
	types := make(map[string]typeDeclaration)
	var constants []constantDeclaration

	for _, filename := range info.GoFiles {
		if filename == output {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(info.Dir, filename), nil, parser.ParseComments)
		if err != nil {
			return sourcePackage{}, err
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			switch general.Tok {
			case token.TYPE:
				collectTypes(types, general)
			case token.CONST:
				collectConstants(&constants, general)
			}
		}
	}

	return sourcePackage{info: info, types: types, consts: resolveConstants(constants)}, nil
}

func collectTypes(types map[string]typeDeclaration, declaration *ast.GenDecl) {
	for _, rawSpec := range declaration.Specs {
		spec := rawSpec.(*ast.TypeSpec)
		doc := commentText(spec.Doc)
		if doc == "" {
			doc = commentText(declaration.Doc)
		}
		types[spec.Name.Name] = typeDeclaration{doc: doc, spec: spec}
	}
}

func collectConstants(constants *[]constantDeclaration, declaration *ast.GenDecl) {
	var previousType string
	var previousValues []ast.Expr

	for _, rawSpec := range declaration.Specs {
		spec := rawSpec.(*ast.ValueSpec)
		typeName := identifierName(spec.Type)
		expressions := spec.Values
		if len(expressions) == 0 {
			typeName = previousType
			expressions = previousValues
		}
		previousType = typeName
		previousValues = expressions

		for index, name := range spec.Names {
			expressionIndex := min(index, len(expressions)-1)
			if expressionIndex < 0 {
				continue
			}
			*constants = append(*constants, constantDeclaration{
				name:       name.Name,
				typeName:   typeName,
				expression: expressions[expressionIndex],
			})
		}
	}
}

func resolveConstants(constants []constantDeclaration) map[string][]enumValue {
	declarations := make(map[string]constantDeclaration, len(constants))
	for _, declaration := range constants {
		declarations[declaration.name] = declaration
	}

	values := make(map[string]string, len(constants))
	resolvingValues := make(map[string]bool)
	var resolveValue func(string) (string, bool)
	resolveValue = func(name string) (string, bool) {
		if value, ok := values[name]; ok {
			return value, true
		}
		declaration, ok := declarations[name]
		if !ok || resolvingValues[name] {
			return "", false
		}
		resolvingValues[name] = true
		value, ok := stringConstant(declaration.expression, resolveValue)
		delete(resolvingValues, name)
		if ok {
			values[name] = value
		}
		return value, ok
	}

	resolvedTypes := make(map[string]string, len(constants))
	resolvingTypes := make(map[string]bool)
	var resolveType func(string) string
	resolveType = func(name string) string {
		if typeName := resolvedTypes[name]; typeName != "" {
			return typeName
		}
		declaration, ok := declarations[name]
		if !ok || resolvingTypes[name] {
			return ""
		}
		resolvingTypes[name] = true
		typeName := declaration.typeName
		if typeName == "" {
			typeName = constantExpressionType(declaration.expression, resolveType)
		}
		delete(resolvingTypes, name)
		resolvedTypes[name] = typeName
		return typeName
	}

	enums := make(map[string][]enumValue)
	for _, declaration := range constants {
		typeName := resolveType(declaration.name)
		value, ok := resolveValue(declaration.name)
		if typeName != "" && ok {
			enums[typeName] = append(enums[typeName], enumValue{name: declaration.name, value: value})
		}
	}
	return enums
}

func constantExpressionType(expression ast.Expr, resolveIdentifier func(string) string) string {
	switch expression := expression.(type) {
	case *ast.CallExpr:
		return identifierName(expression.Fun)
	case *ast.Ident:
		return resolveIdentifier(expression.Name)
	case *ast.ParenExpr:
		return constantExpressionType(expression.X, resolveIdentifier)
	case *ast.UnaryExpr:
		return constantExpressionType(expression.X, resolveIdentifier)
	case *ast.BinaryExpr:
		left := constantExpressionType(expression.X, resolveIdentifier)
		right := constantExpressionType(expression.Y, resolveIdentifier)
		if left == "" || left == right {
			return right
		}
		if right == "" {
			return left
		}
	}
	return ""
}

func stringConstant(expression ast.Expr, resolveIdentifier func(string) (string, bool)) (string, bool) {
	switch expression := expression.(type) {
	case *ast.BasicLit:
		if expression.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(expression.Value)
		return value, err == nil
	case *ast.Ident:
		return resolveIdentifier(expression.Name)
	case *ast.ParenExpr:
		return stringConstant(expression.X, resolveIdentifier)
	case *ast.CallExpr:
		if len(expression.Args) != 1 {
			return "", false
		}
		return stringConstant(expression.Args[0], resolveIdentifier)
	case *ast.BinaryExpr:
		if expression.Op != token.ADD {
			return "", false
		}
		left, leftOK := stringConstant(expression.X, resolveIdentifier)
		right, rightOK := stringConstant(expression.Y, resolveIdentifier)
		return left + right, leftOK && rightOK
	default:
		return "", false
	}
}

func buildMetadata(source sourcePackage, serviceNames, rootTypeNames []string) (generatedPackage, error) {
	metadata := generatedPackage{
		info:     source.info,
		types:    make(map[string]generatedType),
		services: make(map[string]generatedService),
	}
	reachable := make(map[string]bool)

	for _, serviceName := range serviceNames {
		declaration, ok := source.types[serviceName]
		if !ok {
			return generatedPackage{}, fmt.Errorf("service interface %q not found", serviceName)
		}
		service, ok := declaration.spec.Type.(*ast.InterfaceType)
		if !ok {
			return generatedPackage{}, fmt.Errorf("%q is not an interface", serviceName)
		}

		methods := make(map[string]string)
		collectServiceMethods(service, source.types, reachable, methods, make(map[string]bool))
		metadata.services[serviceName] = generatedService{
			description: declaration.doc,
			methods:     methods,
		}
	}
	for _, typeName := range rootTypeNames {
		if _, ok := source.types[typeName]; !ok {
			return generatedPackage{}, fmt.Errorf("root type %q not found", typeName)
		}
		reachable[typeName] = true
	}

	queue := sortedSet(reachable)
	queued := make(map[string]bool, len(queue))
	for _, name := range queue {
		queued[name] = true
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		declaration := source.types[name]
		collectReferencedTypes(declaration.spec.Type, source.types, reachable)
		for _, referencedName := range sortedSet(reachable) {
			if !queued[referencedName] {
				queue = append(queue, referencedName)
				queued[referencedName] = true
			}
		}

		generated := generatedType{description: declaration.doc}
		if structure, ok := declaration.spec.Type.(*ast.StructType); ok {
			generated.fields = structFieldDocs(structure)
		}
		if identifierName(declaration.spec.Type) == "string" {
			generated.enum = uniqueEnumValues(source.consts[name])
		}
		metadata.types[name] = generated
	}

	return metadata, nil
}

func collectServiceMethods(
	service *ast.InterfaceType,
	declarations map[string]typeDeclaration,
	reachable map[string]bool,
	methods map[string]string,
	seen map[string]bool,
) {
	for _, field := range service.Methods.List {
		collectReferencedTypes(field.Type, declarations, reachable)
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				methods[name.Name] = commentText(field.Doc)
			}
			continue
		}

		embeddedName := identifierName(field.Type)
		if embeddedName == "" || seen[embeddedName] {
			continue
		}
		declaration, ok := declarations[embeddedName]
		if !ok {
			continue
		}
		embedded, ok := declaration.spec.Type.(*ast.InterfaceType)
		if !ok {
			continue
		}
		seen[embeddedName] = true
		collectServiceMethods(embedded, declarations, reachable, methods, seen)
	}
}

func uniqueEnumValues(values []enumValue) []enumValue {
	unique := make([]enumValue, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value.value] {
			continue
		}
		seen[value.value] = true
		unique = append(unique, value)
	}
	return unique
}

func collectReferencedTypes(expression ast.Expr, declarations map[string]typeDeclaration, found map[string]bool) {
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if _, local := declarations[identifier.Name]; local {
			found[identifier.Name] = true
		}
		return true
	})
}

func structFieldDocs(structure *ast.StructType) map[string]string {
	fields := make(map[string]string)
	for _, field := range structure.Fields.List {
		doc := fieldComment(field)
		if doc == "" {
			continue
		}
		for _, name := range field.Names {
			fields[name.Name] = doc
		}
	}
	return fields
}

func fieldComment(field *ast.Field) string {
	if doc := commentText(field.Doc); doc != "" {
		return doc
	}
	return commentText(field.Comment)
}

func commentText(comments *ast.CommentGroup) string {
	if comments == nil {
		return ""
	}
	return strings.TrimSpace(comments.Text())
}

func identifierName(expression ast.Expr) string {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func sortedSet(set map[string]bool) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func render(metadata generatedPackage) ([]byte, error) {
	var output bytes.Buffer
	fmt.Fprintln(&output, "// Code generated by lokerpcgen. DO NOT EDIT.")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "package %s\n\n", metadata.info.Name)
	fmt.Fprintln(&output, `import "github.com/LOKE/pkg/lokerpc"`)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "func init() {")
	fmt.Fprintf(&output, "\tlokerpc.RegisterPackageMetadata(%s, lokerpc.PackageMetadata{\n", strconv.Quote(metadata.info.ImportPath))

	fmt.Fprintln(&output, "\t\tTypes: map[string]lokerpc.TypeMetadata{")
	for _, name := range sortedGeneratedTypes(metadata.types) {
		typeMetadata := metadata.types[name]
		if typeMetadata.description == "" && len(typeMetadata.fields) == 0 && len(typeMetadata.enum) == 0 {
			continue
		}
		fmt.Fprintf(&output, "\t\t\t%s: {\n", strconv.Quote(name))
		if typeMetadata.description != "" {
			fmt.Fprintf(&output, "\t\t\t\tDescription: %s,\n", strconv.Quote(typeMetadata.description))
		}
		if len(typeMetadata.fields) > 0 {
			fmt.Fprintln(&output, "\t\t\t\tFields: map[string]string{")
			for _, field := range sortedStringKeys(typeMetadata.fields) {
				fmt.Fprintf(&output, "\t\t\t\t\t%s: %s,\n", strconv.Quote(field), strconv.Quote(typeMetadata.fields[field]))
			}
			fmt.Fprintln(&output, "\t\t\t\t},")
		}
		if len(typeMetadata.enum) > 0 {
			fmt.Fprintln(&output, "\t\t\t\tEnum: []lokerpc.EnumValue{")
			for _, enum := range typeMetadata.enum {
				fmt.Fprintf(&output, "\t\t\t\t\t{Name: %s, Value: %s},\n", strconv.Quote(enum.name), strconv.Quote(enum.value))
			}
			fmt.Fprintln(&output, "\t\t\t\t},")
		}
		fmt.Fprintln(&output, "\t\t\t},")
	}
	fmt.Fprintln(&output, "\t\t},")

	fmt.Fprintln(&output, "\t\tServices: map[string]lokerpc.ServiceMetadata{")
	for _, name := range sortedGeneratedServices(metadata.services) {
		service := metadata.services[name]
		fmt.Fprintf(&output, "\t\t\t%s: {\n", strconv.Quote(name))
		if service.description != "" {
			fmt.Fprintf(&output, "\t\t\t\tDescription: %s,\n", strconv.Quote(service.description))
		}
		fmt.Fprintln(&output, "\t\t\t\tMethods: map[string]string{")
		for _, method := range sortedStringKeys(service.methods) {
			if service.methods[method] != "" {
				fmt.Fprintf(&output, "\t\t\t\t\t%s: %s,\n", strconv.Quote(method), strconv.Quote(service.methods[method]))
			}
		}
		fmt.Fprintln(&output, "\t\t\t\t},")
		fmt.Fprintln(&output, "\t\t\t},")
	}
	fmt.Fprintln(&output, "\t\t},")
	fmt.Fprintln(&output, "\t})")
	fmt.Fprintln(&output, "}")

	return format.Source(output.Bytes())
}

func sortedGeneratedTypes(values map[string]generatedType) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedGeneratedServices(values map[string]generatedService) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
