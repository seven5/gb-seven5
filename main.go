package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	verbose = true
)

func main() {
	project := os.Getenv("GB_PROJECT_DIR")

	//sanity
	if project == "" {
		panic("gb extensions should be launched with GB_PROJECT_DIR set")
	}
	//validate that gopherjs, pagegen are there
	if err := validateExecutablesInPath(project); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	//figure out args, if any
	args := os.Args[1:]
	if len(args) == 0 {
		help()
		os.Exit(0)
	}

	//walk each arg, assuming that they are golang package specs
	for _, arg := range args {

		//make sure everything is where we expect within arg
		if err := validateProjectStructure(project, arg); err != nil {
			os.Exit(1)
		}

		//gopherjs creates the js code
		if err := gopherjsCompilation(project, arg); err != nil {
			os.Exit(1)
		}

		//pagegen creates the HTML pages
		if err := pageGeneration(project, arg); err != nil {
			os.Exit(1)
		}
	}
}

func pageGeneration(project string, arg string) error {
	templatePath := constructTemplatesPath(project, arg)

	jsonFiles := []string{}
	htmlFiles := []string{}
	err := filepath.Walk(templatePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error walking %s: %v\n", path, err)
			return err
		}
		//ignore the support dir
		if info.IsDir() && info.Name() == "support" {
			return filepath.SkipDir
		}
		//make sure that for each json there is an HTML
		if strings.HasSuffix(info.Name(), ".json") {
			parent := filepath.Dir(path)
			root := strings.TrimSuffix(info.Name(), ".json")
			_, err := os.Stat(filepath.Join(parent, root+".html"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "unable to find corresponding html file for json file %s\n", path)
				return fmt.Errorf("no html file for %s", path)
			}
			jsonFiles = append(jsonFiles, path)
			htmlFiles = append(htmlFiles, filepath.Join(parent, root+".html"))
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to walk directory %s: %v", templatePath, err)
		return err
	}

	for i, jsonFile := range jsonFiles {
		if !strings.HasPrefix(jsonFile, constructTemplatesPath(project, arg)) {
			panic(fmt.Sprintf("unable to understand json path %s in template dir %s",
				jsonFile, constructTemplatesPath(project, arg)))
		}
		html := strings.TrimPrefix(htmlFiles[i], constructTemplatesPath(project, arg))
		json := strings.TrimPrefix(jsonFile, constructTemplatesPath(project, arg))
		if err := launchPagegen("support",
			constructTemplatesPath(project, arg),
			html, json,
			filepath.Join(constructStaticEnglishPath(project, arg), html)); err != nil {
			return err
		}
	}
	return nil
}

func gopherjsCompilation(project string, arg string) error {
	//this the full path to the package from arg
	dir := constructClientPackagePath(project, arg)

	//find the gofiles in the package
	gofiles, err := iterateDirs([]string{dir})
	if err != nil {
		return err
	}

	//find the gofiles that have a main()
	pages := []string{}
	for _, gofile := range gofiles {
		hasMain, err := hasMainFunc(gofile)
		if err != nil {
			return err
		}
		if hasMain {
			pages = append(pages, gofile)
		}
	}

	//walk each page, compiling to the static/en/web
	for _, page := range pages {
		if !strings.HasPrefix(page, constructClientPackagePath(project, arg)) {
			panic(fmt.Sprintf("unable to understand page path %s in package %s",
				page, constructClientPackagePath(project, arg)))
		}
		suffix := strings.TrimPrefix(page, constructClientPackagePath(project, arg))
		target := filepath.Join(constructStaticEnglishPath(project, arg), suffix)
		if err := launchGopherjs(project, "build", "-m", "-o", target, page); err != nil {
			return err
		}
	}

	return nil
}

func iterateDirs(dirs []string) ([]string, error) {
	gofiles := []string{}
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				fmt.Fprintf(os.Stderr, "error walking %s: %v\n", path, err)
				return err
			}
			if strings.HasSuffix(info.Name(), ".go") {
				gofiles = append(gofiles, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return gofiles, nil
}

func hasMainFunc(path string) (bool, error) {
	fset := token.NewFileSet() // positions are relative to fset

	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing %s: %v", path, err)
		return false, err
	}

	for _, decl := range f.Decls {
		switch x := decl.(type) {
		case *ast.FuncDecl:
			if x.Name.String() == "main" {
				return true, nil
			}
		}
	}
	return false, nil
}

func launchGopherjs(projectDir string, args ...string) error {
	cmd := exec.Command("gopherjs", args...)
	vendor := projectDir + string(os.PathSeparator) + "vendor"
	bothDirs := projectDir + string(os.PathListSeparator) + vendor
	cmd.Env = append(os.Environ(), "GOPATH="+bothDirs)
	out, err := cmd.CombinedOutput()
	fmt.Printf("%s", string(out))
	return err
}

func launchPagegen(supportPath, templatesPath, htmlInFile, jsonFile, htmlOutFile string) error {
	fmt.Printf("XXXIES::: %s, %s, %s, %s\n", supportPath, templatesPath, htmlInFile, jsonFile)
	cmd := exec.Command("pagegen", "--support", supportPath, "--dir", templatesPath, "--start",
		htmlInFile, "--json", jsonFile)
	out, err := cmd.Output()
	if err != nil {
		if execError, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "%s\n", string(execError.Stderr))
			return execError
		}
		fmt.Fprintf(os.Stderr, "Unable to start pagegen process: %v", err)
		return err
	}
	file, err := os.Create(htmlOutFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to create output file %s: %v", htmlOutFile, err)
		return err
	}
	buff := bytes.NewBuffer(out)
	_, err = io.Copy(file, buff)
	return err
}

func help() {
	fmt.Printf("gb seven5 requires a package name to build client software from\n")
}

//
// SUPPORT FUNCS
//

func constructClientPackagePath(project string, arg string) string {
	return filepath.Join(project, "src", arg, "client")
}
func constructPagesPath(project string, arg string) string {
	return filepath.Join(project, "src", arg, "pages")
}
func constructTemplatesPath(project string, arg string) string {
	return filepath.Join(project, "src", arg, "pages", "template")
}
func constructSupportPath(project string, arg string) string {
	return filepath.Join(project, "src", arg, "pages", "template", "support")
}

func constructStaticEnglishPath(project string, arg string) string {
	return filepath.Join(project, "src", arg, "static", "en", "web")
}

func validateClientPackage(projectDir string, arg string) error {
	path := constructClientPackagePath(projectDir, arg)
	_, err := os.Stat(path)
	return err
}
func validatePagesDir(projectDir string, arg string) error {
	path := constructPagesPath(projectDir, arg)
	_, err := os.Stat(path)
	return err
}

func validateStaticEnglishDir(projectDir string, arg string) error {
	path := constructStaticEnglishPath(projectDir, arg)
	_, err := os.Stat(path)
	return err
}

func validateExecutablesInPath(projectDir string) error {
	cmd := exec.Command("gopherjs")
	cmd.Env = append(os.Environ(), "GOPATH="+projectDir)
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("pagegen")
	return cmd.Run()
}

func validateProjectStructure(project string, arg string) error {
	//validate that the packages provided have a client subpackage
	//and the static/en/web directory, as expected
	if err := validateClientPackage(project, arg); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find client package in %s\n",
			constructClientPackagePath(project, arg))
		return err
	}
	if err := validateStaticEnglishDir(project, arg); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find static/en/web directory, expected it to be %s\n",
			constructStaticEnglishPath(project, arg))
		return err
	}
	//make sure it has the pages dir
	if err := validatePagesDir(project, arg); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to find pages directory, expected it to be %s\n",
			constructPagesPath(project, arg))
		return err
	}
	return nil
}
