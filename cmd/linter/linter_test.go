package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := t.TempDir()

	// Создаём структуру testdata/src/a
	pkgDir := filepath.Join(testdata, "src", "a")
	err := os.MkdirAll(pkgDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	badGoCode := `package a

import (
	"log"
	"os"
)

func BadFunc1() {
	panic("error") // want "использование встроенной функции panic"
}

func BadFunc2() {
	log.Fatal("error") // want "вызов log.Fatal вне функции main пакета main"
}

func BadFunc3() {
	log.Fatalf("error: %v", "something") // want "вызов log.Fatalf вне функции main пакета main"
}

func BadFunc4() {
	log.Fatalln("error") // want "вызов log.Fatalln вне функции main пакета main"
}

func BadFunc5() {
	os.Exit(1) // want "вызов os.Exit вне функции main пакета main"
}

func GoodFunc() {
	log.Println("info message")
}
`

	err = os.WriteFile(filepath.Join(pkgDir, "bad.go"), []byte(badGoCode), 0644)
	if err != nil {
		t.Fatal(err)
	}

	analysistest.Run(t, testdata, Analyzer, "a")
}

func TestAnalyzerMainPackage(t *testing.T) {
	testdata := t.TempDir()

	// Создаём структуру testdata/src/mainpkg
	pkgDir := filepath.Join(testdata, "src", "mainpkg")
	err := os.MkdirAll(pkgDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	mainGoCode := `package main

import (
	"log"
	"os"
)

func helper() {
	panic("error") // want "использование встроенной функции panic"
	log.Fatal("error") // want "вызов log.Fatal вне функции main пакета main"
	os.Exit(1) // want "вызов os.Exit вне функции main пакета main"
}

func main() {
	// Это допустимо
	if false {
		log.Fatal("ok")
		os.Exit(0)
	}
}
`

	err = os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte(mainGoCode), 0644)
	if err != nil {
		t.Fatal(err)
	}

	analysistest.Run(t, testdata, Analyzer, "mainpkg")
}
