// Copyright 2000-2022 JetBrains s.r.o. and contributors. Use of this source code is governed by the Apache 2.0 license.
package main

// Collect stats on JVP Packages for JPS modules.
// The results of `go run ./scan_packages.go -gs -d ./platform`
//  available at https://jb.gg/platform-packages

// It gets the next modules right (by parsing .iml)
//  * platform/object-serializer/annotations
//  * platform/structuralsearch/source
//  * platform/util/concurrency (and ui and util, wich are in the same root)
// It also skips
//  1. platform/built-in-server/client/node-rpc-client/intellij.nodeRpcClient.iml source dirs:0
//    a default srcDir for `<module type="JAVA_MODULE" ..>` vs `<module type="WEB_MODULE" ..>?`
//    coupled with <orderEntry type="sourceFolder" forTests="false" />
//  2. platform/icons/intellij.platform.icons.iml                             source dirs:2
//  3. platform/platform-resources/intellij.platform.resources.iml            source dirs:1
//  4. platform/platform-resources-en/intellij.platform.resources.en.iml      source dirs:1
//  5. platform/workspaceModel/storage/testEntities/intellij.platform.workspaceModel.storage.testEntities.iml source dirs:2

import (
	"bufio"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const spaceURL = "https://jetbrains.team/p/ij/repositories/community/files/"

var (
	dirFlag = flag.String("d", "", "dir to scan for packages")
	mdFlag  = flag.Bool("md", false, "format output as Markdown")
	gsFlag  = flag.Bool("gs", false, "format output as a Spreadsheet")
)

// TODO(bzz):
//  * option to output a table in .md format
// 		* cloumn: mark packages \w existing JavaDoc (road works emoji)
//      * column: mark modules (or packages?) \w readme
//      * column: "doc coverage"? quantify presence of documentation
//  * get the commit sha (git rev-parse ?)

//  * srcDir: does module type="JAVA_MODULE" has any defaults?

type pkg struct {
	module   string // path to .iml file
	srcDir   string // path to src/ or <sourceFolder .../> from .iml
	pkgDir   string // path to package
	name     string // as in `import ...`
	doc      string // existing documentation
	files    []string
	filesCnt map[string]int // number of .kt and .java files
}

func main() {
	flag.Parse()
	if *dirFlag == "" {
		flag.Usage()
		return
	}

	ext := ".iml"
	modulesPaths, err := findModulesPaths(*dirFlag, ext)
	if err != nil {
		fmt.Printf("error walking the path %q looking for *%q: %v\n", *dirFlag, ext, err)
		return
	}

	srcDirPaths, err := grepXMLForSrcDirPaths(modulesPaths)
	panicIfError(err)

	// collect the packages
	pkgs := map[string]*pkg{}
	for srcDir, mod := range srcDirPaths {
		err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}

			if strings.HasPrefix(filepath.Base(path), "_") { // templates for some code-gen?
				// platform/testFramework/src/{_FirstInSuiteTest.java, _LastInSuiteTest.java}
				return nil
			}
			if di, _ := d.Info(); di.Size() == 0 { // skip empty files
				// platform/testFramework/src/com/intellij/codeInsight/codeVision/CodeVisionTestCase.kt
				return nil
			}

			pkgDir := filepath.Dir(path)
			if existingPkg, ok := pkgs[pkgDir]; ok {
				if strings.HasSuffix(path, "package-info.java") || strings.HasSuffix(path, "package.html") {
					existingPkg.doc = path
				}
				return nil // skip the rest of the files for a known package
			}

			if strings.HasSuffix(path, ".java") || strings.HasSuffix(path, ".kt") {
				pkgName, err := readPkgNameFromFirstLines(path, 100)
				if err != nil {
					return err
				}

				newPkg := &pkg{module: mod, srcDir: srcDir, pkgDir: pkgDir, name: pkgName}
				if strings.HasSuffix(path, "package-info.java") || strings.HasSuffix(path, "package.html") {
					newPkg.doc = path
				}
				pkgs[pkgDir] = newPkg
			}
			return nil
		})
		panicIfError(err)
	}

	// collect the files
	readPkgDirsToCollectFiles(pkgs)

	// print: header
	fields := []string{"files", ".java", ".kt", "module", "package", "documentation"}
	if *gsFlag {
		fmt.Println(strings.Join(fields, "\t"))
	}
	if *mdFlag {
		fmt.Println(strings.Join(fields, " | "))
		fmt.Print("--")
		for i := 0; i < (len(fields) - 1); i++ {
			fmt.Print("|--")
		}
		fmt.Println()
	}

	// print: body
	for _, pkg := range pkgs {
		pkgLink := spaceURL + pkg.pkgDir
		fmtPkgLink := pkg.pkgDir

		docSign := ""
		if strings.HasSuffix(pkg.doc, ".html") {
			docSign = "🚧"
		} else if strings.HasSuffix(pkg.doc, ".java") {
			docSign = "✅"
		}

		if *gsFlag {
			fmtPkgLink = fmt.Sprintf(`=HYPERLINK("%s","%s")`, pkgLink, pkg.name)

			fmtDocLink := ""
			if docSign != "" {
				fmtDocLink = fmt.Sprintf(`=HYPERLINK("%s","%s")`, spaceURL+pkg.doc, docSign)
			}
			fmt.Printf("%d\t%d\t%d\t%s\t%s\t%s\n", len(pkg.files), pkg.filesCnt[".java"], pkg.filesCnt[".kt"], pkg.module, fmtPkgLink, fmtDocLink)
		} else if *mdFlag {
			fmtPkgLink = fmt.Sprintf("[%s](%s)", pkg.name, pkgLink)
			fmt.Printf("%-3d | %-3d | %-3d | %-50s | %s\n", len(pkg.files), pkg.filesCnt[".java"], pkg.filesCnt[".kt"], pkg.module, fmtPkgLink)
		} else {
			fmt.Printf("%d\t%d\t%d\t%s\t%s\n", len(pkg.files), pkg.filesCnt[".java"], pkg.filesCnt[".kt"], fmtPkgLink, docSign+" "+pkg.doc)
		}

	}

	// if *csvFlag {
	// TODO print abs path all the files (to feed into python indexer)
	// }
}

func readPkgNameFromFirstLines(path string, n int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	i := 0
	pkgName := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && i < n { // read first 100 lines
		if strings.HasPrefix(scanner.Text(), "package ") {
			ss := strings.Fields(scanner.Text())
			if len(ss) != 2 {
				fmt.Fprintf(os.Stderr, "fail to get package name for %q\n", path)
			}
			pkgName = strings.TrimRight(ss[1], ";")
			break
		}
		i++
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "fail reading file %q :%v\n", path, err)
		return "", err
	}

	return pkgName, nil
}

// readPkgDirsToCollectFiles updates .files & .fileCnt for each package in a map by reading .pkgDir from FS once.
func readPkgDirsToCollectFiles(pkgs map[string]*pkg) {
	for pkgDir, pkg := range pkgs {
		files, err := os.ReadDir(pkgDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to travers fs for package %q, %+v", pkgDir, err)
			continue
		}

		filesCnt := map[string]int{}
		for _, f := range files {
			fName := f.Name()
			if !f.IsDir() && (strings.HasSuffix(fName, ".java") || strings.HasSuffix(fName, ".kt")) {
				pkg.files = append(pkg.files, fName)
				filesCnt[filepath.Ext(fName)] = filesCnt[filepath.Ext(fName)] + 1
			}
		}
		pkg.filesCnt = filesCnt
		// fmt.Printf("%d\t%d\t%d\t%s\n", len(pkg.files), filesCnt[".java"], filesCnt[".kt"], pkgDir)
	}
}

// grepXMLForSrcDirPaths return map of source Dir root -> .iml module
func grepXMLForSrcDirPaths(modulesPaths []string) (map[string]string, error) {
	srcDirs := make(map[string]string, len(modulesPaths))
	for _, mp := range modulesPaths { // parse XMLs
		module, err := newModuleFromXMLFile(mp)
		if err != nil {
			return nil, err
		}

		//// .srcDirURL() errors on n == 0
		// n := module.srcDirCount()
		// if n == 0 { // 145 -> 140
		// 	continue
		// }

		srcDirURL, err := module.srcDirURL()
		if err != nil {
			// fmt.Fprintf(os.Stderr, "%s has no source dir", mp)
			continue
		}
		srcDir := filepath.Join(filepath.Dir(mp), filepath.Base(srcDirURL))
		srcDirs[srcDir] = mp
		// fmt.Printf("%-76s  <sourceFolder/>:%+v, actual:%d, %s\n", mp, len(module.Component.SourceFolders), n, srcDir)
	}
	return srcDirs, nil
}

// newModuleFromXMLFile reads given XML file and parses it as a module struct.
func newModuleFromXMLFile(path string) (*module, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %v\n", path, err)
	}

	var m module
	if err := xml.Unmarshal(blob, &m); err != nil {
		return nil, fmt.Errorf("error parsing XML %q: %v\n", path, err)
	}
	return &m, nil
}

// findModulesPaths traverses filesystem from the rootDir, skipping test directories,
// returning all files with the given extension.
func findModulesPaths(rootDir, fileExt string) ([]string, error) {
	skipDirs := map[string]bool{
		"test": true, "tests": true, "testData": true, "testSources": true, "testSource": true, "testSrc": true, "testResources": true,
		"gen": true, "generated": true,
		"resources":     true,
		"build-scripts": true, // TODO(bzz): confirm, filters 5 modules
	}
	testModules := regexp.MustCompile(fmt.Sprintf("[tT]ests%s$", fileExt))

	var modules []string
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()]) {
			return filepath.SkipDir
		}

		if strings.HasSuffix(d.Name(), fileExt) && !testModules.MatchString(d.Name()) {
			modules = append(modules, path)
		}
		return nil
	})
	return modules, err
}

// .iml XML schema
type module struct {
	XMLName   xml.Name `xml:"module"`
	Component struct { // can this be removed? not really, as we need specificaly the one with `name="NewModuleRootManager"`
		// see ./platform/remoteDev-util/intellij.remoteDev.util.iml for multiple ones + type="GENERAL_MODULE"
		XMLName       xml.Name `xml:"component"`
		Name          string   `xml:"name,attr,omitempty"` // TODO(bzz): convert to slice and pick only NewModuleRootManager
		SourceFolders []srcDir `xml:"content>sourceFolder"`
	} `xml:"component"`
}

func (m *module) srcDirCount() int {
	n := 0
	for _, d := range m.Component.SourceFolders {
		if !d.Generated && !d.IsTest && !d.isResource() { // 150 -> 145
			n++
		}
	}
	return n
}

func (m *module) srcDirURL() (string, error) {
	for _, d := range m.Component.SourceFolders {
		if !d.Generated && !d.IsTest && !d.isResource() {
			return d.Url, nil
		}
	}
	return "", errors.New("no <sourceFolder /> that is not test or resource")
}

type srcDir struct {
	XMLName   xml.Name `xml:"sourceFolder"`
	Url       string   `xml:"url,attr"`
	IsTest    bool     `xml:"isTestSource,attr,omitempty"`
	Generated bool     `xml:"generated,attr,omitempty"`
	Type      string   `xml:"type,attr"`
}

func (sd *srcDir) isResource() bool {
	return strings.HasSuffix(sd.Type, "-resource")
}

// helpers
func panicIfError(err error) {
	if err == nil {
		return
	}

	panic(err)
}
