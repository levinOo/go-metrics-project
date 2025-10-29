package main

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "exitcheck",
	Doc:  "проверяет использование panic, os.Exit и log.Fatal вне main пакета main",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			if ident, ok := call.Fun.(*ast.Ident); ok {
				if ident.Name == "panic" {
					pass.Reportf(call.Pos(), "использование встроенной функции panic")
				}
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			pkgName := x.Name
			funcName := sel.Sel.Name

			if (pkgName == "log" && isFatalFunc(funcName)) ||
				(pkgName == "os" && funcName == "Exit") {
				// Проверяем, находимся ли в main.main
				if !isInMainFunc(pass, call) {
					pass.Reportf(call.Pos(),
						"вызов %s.%s вне функции main пакета main",
						pkgName, funcName)
				}
			}

			return true
		})
	}

	return nil, nil
}

func isFatalFunc(name string) bool {
	return name == "Fatal" || name == "Fatalf" || name == "Fatalln"
}

// isInMainFunc проверяет, находится ли вызов внутри функции main пакета main
func isInMainFunc(pass *analysis.Pass, call *ast.CallExpr) bool {
	// Проверяем, что это пакет main
	if pass.Pkg.Name() != "main" {
		return false
	}

	for _, file := range pass.Files {
		var inMain bool
		ast.Inspect(file, func(n ast.Node) bool {
			funcDecl, ok := n.(*ast.FuncDecl)
			if !ok {
				return true
			}

			if funcDecl.Name.Name == "main" && funcDecl.Recv == nil {
				if funcDecl.Pos() <= call.Pos() && call.End() <= funcDecl.End() {
					inMain = true
					return false
				}
			}
			return true
		})
		if inMain {
			return true
		}
	}

	return false
}
