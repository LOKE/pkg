package lokerpc

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

type pkgDocs struct {
	types map[string]typeDoc
}

type typeDoc struct {
	doc    string
	fields map[string]string
}

type dirEntry struct {
	docs *pkgDocs
	once sync.Once
	err  error
}

var (
	docCacheMu sync.RWMutex
	docCache   = map[string]*dirEntry{}
)

// pkgSourceDir returns the on-disk directory for a package import path.
// Only succeeds for packages within the main module.
// Priority: LOKERPC_SOURCE_ROOT env var → runtime.Caller stack-walk → os.Getwd() walk-up
func pkgSourceDir(pkgPath string) (string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("build info unavailable")
	}

	mainPath := info.Main.Path
	if mainPath == "" || mainPath == "command-line-arguments" {
		return "", fmt.Errorf("no main module path in build info")
	}

	if !strings.HasPrefix(pkgPath, mainPath+"/") && pkgPath != mainPath {
		return "", fmt.Errorf("package %s is not in main module %s", pkgPath, mainPath)
	}

	rel := strings.TrimPrefix(pkgPath, mainPath)
	rel = strings.TrimPrefix(rel, "/")

	// Try LOKERPC_SOURCE_ROOT env var first
	if root := os.Getenv("LOKERPC_SOURCE_ROOT"); root != "" {
		return filepath.Join(root, filepath.FromSlash(rel)), nil
	}

	// Try runtime.Caller stack-walk, verifying the found root declares mainPath.
	if root, ok := findSourceRoot(mainPath); ok {
		return filepath.Join(root, filepath.FromSlash(rel)), nil
	}

	// Fall back to os.Getwd() walk-up, same module verification.
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := findModuleRoot(cwd, mainPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

// findGoModDir walks up from dir looking for any go.mod.
func findGoModDir(dir string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

// findModuleRoot walks up from dir looking for a go.mod that declares modulePath.
func findModuleRoot(dir, modulePath string) (string, error) {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if goModDeclares(dir, modulePath) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("module root for %s not found", modulePath)
		}
		dir = parent
	}
}

// goModDeclares reports whether the go.mod in dir declares the given module path.
func goModDeclares(dir, modulePath string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range strings.SplitN(string(data), "\n", 20) {
		if strings.TrimSpace(line) == "module "+modulePath {
			return true
		}
	}
	return false
}

// findSourceRoot walks the call stack looking for a frame whose module root
// declares mainModPath. Module cache paths are skipped early; all others are
// checked via goModDeclares so stdlib and dependency frames are rejected too.
func findSourceRoot(mainModPath string) (string, bool) {
	pcs := make([]uintptr, 20)
	n := runtime.Callers(1, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	seen := map[string]bool{}
	for {
		frame, more := frames.Next()
		file := frame.File
		if file != "" && !strings.Contains(file, "/pkg/mod/") {
			dir := filepath.Dir(file)
			if root, err := findGoModDir(dir); err == nil && !seen[root] {
				seen[root] = true
				if goModDeclares(root, mainModPath) {
					return root, true
				}
			}
		}
		if !more {
			break
		}
	}
	return "", false
}

// loadPkgDocs loads (with caching) the docs for a package directory.
// Single parse per directory guaranteed via sync.Once.
func loadPkgDocs(dir string) (*pkgDocs, error) {
	docCacheMu.Lock()
	e, ok := docCache[dir]
	if !ok {
		e = &dirEntry{}
		docCache[dir] = e
	}
	docCacheMu.Unlock()

	e.once.Do(func() {
		e.docs, e.err = parsePkgDocs(dir)
	})
	return e.docs, e.err
}

// parsePkgDocs parses all .go files in dir and extracts struct/field docs.
func parsePkgDocs(dir string) (*pkgDocs, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	result := &pkgDocs{types: map[string]typeDoc{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ParseComments)
		if err != nil {
			continue // skip unparseable files
		}
		extractFileDocs(f, result)
	}
	return result, nil
}

func extractFileDocs(file *ast.File, result *pkgDocs) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}

			td := typeDoc{fields: map[string]string{}}

			// Prefer TypeSpec.Doc, fall back to GenDecl.Doc
			if ts.Doc != nil {
				td.doc = strings.TrimSpace(ts.Doc.Text())
			} else if gd.Doc != nil {
				td.doc = strings.TrimSpace(gd.Doc.Text())
			}

			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					continue // skip embedded fields
				}

				var comment string
				if field.Doc != nil {
					comment = strings.TrimSpace(field.Doc.Text())
				} else if field.Comment != nil {
					comment = strings.TrimSpace(field.Comment.Text())
				}
				if comment == "" {
					continue
				}

				key := fieldJSONKey(field)
				if key != "" {
					td.fields[key] = comment
				}
			}

			result.types[ts.Name.Name] = td
		}
	}
}

// fieldJSONKey returns the JSON key for a struct field (same logic as TypeSchema/parseTag).
func fieldJSONKey(field *ast.Field) string {
	if field.Tag != nil {
		tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
		name, _ := parseTag(tag.Get("json"))
		if name != "" && name != "-" {
			return name
		}
	}
	if len(field.Names) > 0 {
		return field.Names[0].Name
	}
	return ""
}
