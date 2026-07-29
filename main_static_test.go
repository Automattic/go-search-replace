package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestMainAvoidsForbiddenConversion(t *testing.T) {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, basePath, func(file os.FileInfo) bool {
		name := file.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	mainPackage, ok := packages["main"]
	if !ok {
		t.Fatal("package main not found")
	}

	forbiddenImport := "un" + "safe"
	forbiddenHelper := forbiddenImport + "Get" + "String"

	for _, mainFile := range mainPackage.Files {
		for _, importSpec := range mainFile.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}

			if importPath == forbiddenImport {
				t.Fatalf("%s imports forbidden package %q", fileSet.Position(importSpec.Pos()), importPath)
			}
		}

		ast.Inspect(mainFile, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if ok && ident.Name == forbiddenHelper {
				t.Errorf("%s references forbidden helper %q", fileSet.Position(ident.Pos()), ident.Name)
			}

			return true
		})
	}
}
