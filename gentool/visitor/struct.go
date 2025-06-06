package visitor

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"slices"
	"strings"
	"text/template"
)

const (
	StructFlag          = "StructInfo"
	DependImportsFlag   = "DependImports"
	PkgFlag             = "Pkg"
	OtherMethodsFlag    = "OtherMethods"
	MethodSignatureTemp = "{{$method.Name}}({{range $i, $param := $method.Params}}{{if gt $i 0}},{{end}}{{$param.Name}} {{if $param.IsPointer}}*{{end}}{{$param.TypeName}}{{end}}) ({{range $i, $param := $method.Results}}{{if gt $i 0}},{{end}}{{$param.Name}} {{if $param.IsPointer}}*{{end}}{{$param.TypeName}}{{end}})"
	MethodCallTemp      = "{{$method.Name}}({{range $i, $param := $method.Params}}{{if gt $i 0}},{{end}}{{$param.Name}} {{end}})"
)

type ScanStruct func(signature *StructInfo) bool
type ScanMethod func(signature *MethodSignature) bool

func GenWithTemplate(inputFile, outputFile, temp string, scanStruct ScanStruct, scanMethod ScanMethod) {
	src, err := BuildSrcWithTemplate(inputFile, temp, scanStruct, scanMethod)
	gofile, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer gofile.Close()
	_, err = gofile.Write(src)
	if err != nil {
		panic(err)
	}
}

// BuildSrcWithTemplate 通过Scan方法和temp模板代码构建源码
func BuildSrcWithTemplate(inputFile, temp string, scanStruct ScanStruct, scanMethod ScanMethod) ([]byte, error) {
	visitor := ScanInterface(inputFile, scanStruct, scanMethod)
	fs, err := template.New("").Parse(temp)
	if err != nil {
		return nil, err
	}
	buff := bytes.NewBufferString("")
	err = fs.Execute(buff, visitor)
	if err != nil {
		return nil, err
	}
	//格式化
	return format.Source(buff.Bytes())
}

// ScanInterface 扫描go文件中的接口，目标为出现了{keyword}注释的struct，并且只扫描目标方法集scanMethods
func ScanInterface(inputFile string, scanStruct ScanStruct, scanMethod ScanMethod) *StructVisitor {
	fileSet := token.NewFileSet()
	f, err := parser.ParseFile(fileSet, inputFile, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	visitor := StructVisitor{
		scanStruct: scanStruct,
		scanMethod: scanMethod,
		fileSet:    fileSet,
		Targets:    make([]*StructInfo, 0),
	}
	ast.Walk(&visitor, f)
	visitor.setDependImports()
	return &visitor
}

type StructVisitor struct {
	scanStruct    ScanStruct
	scanMethod    ScanMethod
	fileSet       *token.FileSet
	Pkg           string
	Imports       []*ImportInfo
	Targets       []*StructInfo
	DependImports []*ImportInfo
}

type StructInfo struct {
	Name          string
	Doc           string
	TargetMethods []*MethodSignature
	AllMethods    []*MethodSignature
	OtherMethods  []*MethodSignature
	DependImports []*ImportInfo
}

type MethodSignature struct {
	Name          string
	Doc           string
	Params        []*ParamSignature
	Results       []*ParamSignature
	DependFlags   []string
	DependImports []*ImportInfo
	CtxParamName  *string
	ReturnErr     bool
}

type ParamSignature struct {
	Name      string
	TypeName  string
	IsPointer bool
}

type ImportInfo struct {
	Alias string
	Pkg   string
}

func (receiver *StructVisitor) Visit(node ast.Node) ast.Visitor {
	imports := make([]*ImportInfo, 0)
	spliter := regexp.MustCompile(`\s+`)
	switch node.(type) {
	// root
	case *ast.File:
		receiver.Pkg = node.(*ast.File).Name.Name
	case *ast.GenDecl:
		genDecl := node.(*ast.GenDecl)
		if genDecl.Tok == token.IMPORT {
			// import
			specs := genDecl.Specs
			for _, spec := range specs {
				value := strings.TrimSpace(spec.(*ast.ImportSpec).Path.Value)

				importInfo := ImportInfo{}
				split := spliter.Split(value, 2)
				if len(split) == 2 {
					importInfo.Alias = split[0]
					value = split[1]
				}
				if strings.Contains(value, "\"") {
					value = strings.Replace(value, "\"", "", -1)
				}
				importInfo.Pkg = value
				imports = append(imports, &importInfo)
			}
			receiver.Imports = imports
		} else if genDecl.Tok == token.TYPE {
			// type
			if genDecl.Doc != nil && genDecl.Doc.List != nil {
				// 遍历注释
				doc := genDecl.Doc.Text()
				specs := genDecl.Specs
				for _, spec := range specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						// type
						name := typeSpec.Name.Name
						proxyTarget := StructInfo{
							Name: name,
							Doc:  doc,
						}
						if receiver.scanStruct(&proxyTarget) {
							// collect target methods sign
							targetMethods := make([]*MethodSignature, 0)
							allMethods := make([]*MethodSignature, 0)
							otherMethods := make([]*MethodSignature, 0)
							if interfaceType, ok := typeSpec.Type.(*ast.InterfaceType); ok {
								methods := interfaceType.Methods
								for _, method := range methods.List {
									if len(method.Names) == 1 {
										if funcType, ok := method.Type.(*ast.FuncType); ok {
											methodSign := parserMethod(funcType)
											methodSign.Doc = method.Doc.Text()
											methodSign.Name = method.Names[0].Name
											allMethods = append(allMethods, methodSign)
											if receiver.scanMethod(methodSign) {
												targetMethods = append(targetMethods, methodSign)
											} else {
												otherMethods = append(otherMethods, methodSign)
											}
										}
									}
								}
							}
							proxyTarget.TargetMethods = targetMethods
							proxyTarget.AllMethods = allMethods
							proxyTarget.OtherMethods = otherMethods
							receiver.Targets = append(receiver.Targets, &proxyTarget)
						}
					}
				}
			}
		}
	}
	// get targets imports
	return receiver
}

func (receiver *StructVisitor) setDependImports() {
	dependImportMap := make(map[string]*ImportInfo, 0)
	for _, imp := range receiver.Imports {
		var dependName string
		if imp.Alias != "" {
			dependName = imp.Alias
		} else {
			split := strings.Split(imp.Pkg, "/")
			dependName = split[len(split)-1]
		}
		dependImportMap[dependName] = imp
	}

	allDependImports := make([]*ImportInfo, 0)
	for _, target := range receiver.Targets {
		structDependImports := make([]*ImportInfo, 0)
		for _, method := range target.AllMethods {
			methodDependImports := make([]*ImportInfo, 0)
			for _, flag := range method.DependFlags {
				if info, ok := dependImportMap[flag]; ok {
					if !slices.Contains(methodDependImports, info) {
						methodDependImports = append(methodDependImports, info)
						if !slices.Contains(structDependImports, info) {
							structDependImports = append(structDependImports, info)
							if !slices.Contains(allDependImports, info) {
								allDependImports = append(allDependImports, info)
							}
						}
					}
				}
			}
			method.DependImports = methodDependImports
		}
		target.DependImports = allDependImports
	}

	receiver.DependImports = allDependImports
}

func parserMethod(funcType *ast.FuncType) *MethodSignature {
	signature := MethodSignature{}
	dependFlags := make([]string, 0)
	params := funcType.Params
	paramSignatures := make([]*ParamSignature, len(params.List))
	for i, param := range params.List {
		paramSign, depend := parserParam(param)
		if paramSign.TypeName == "context.Context" {
			signature.CtxParamName = &paramSign.Name
		}
		paramSignatures[i] = paramSign
		if depend != nil {
			dependFlags = append(dependFlags, *depend)
		}
	}

	results := funcType.Results
	resultSignatures := make([]*ParamSignature, len(results.List))
	for i, result := range results.List {
		paramSign, depend := parserParam(result)
		if paramSign.TypeName == "error" {
			signature.ReturnErr = true
		}
		resultSignatures[i] = paramSign
		if depend != nil {
			dependFlags = append(dependFlags, *depend)
		}
	}

	signature.Params = paramSignatures
	signature.Results = resultSignatures
	signature.DependFlags = dependFlags
	return &signature
}

func parserParam(param *ast.Field) (paramSign *ParamSignature, depend *string) {
	name := ""
	if len(param.Names) == 1 {
		name = param.Names[0].Name
	}
	typeName := ""
	paramType := param.Type
	finalTypeName, isPointer, depend := parserExpr(typeName, paramType)
	paramSign = &ParamSignature{
		Name:      name,
		TypeName:  finalTypeName,
		IsPointer: isPointer,
	}
	return
}

func parserExpr(typeName0 string, paramType ast.Expr) (typeName string, isPointer bool, depend *string) {
	switch paramType.(type) {
	case *ast.Ident:
		typeName = typeName0 + paramType.(*ast.Ident).Name
		break
	case *ast.SelectorExpr:
		expr := paramType.(*ast.SelectorExpr)
		preName := expr.X.(*ast.Ident).Name
		depend = &preName
		typeName = typeName0 + preName + "." + expr.Sel.Name
		break
	case *ast.StarExpr:
		starExpr := paramType.(*ast.StarExpr)
		if typeName0 == "" {
			isPointer = true
			typeName, _, depend = parserExpr(typeName0, starExpr.X)
		} else {
			typeName, _, depend = parserExpr(typeName0+"*", starExpr.X)
		}
		break
	case *ast.ArrayType:
		arrType := paramType.(*ast.ArrayType)
		typeName0 += "[]"
		typeName, _, depend = parserExpr(typeName0, arrType.Elt)
		break
	case *ast.MapType:
		mapType := paramType.(*ast.MapType)
		typeName, _, depend = parserExpr("map[", mapType.Key)
		typeName += "]"
		typeName, _, depend = parserExpr(typeName, mapType.Value)
		break
	default:
		panic(fmt.Sprintf("unknown param type:%v", paramType))
	}
	return
}

func ExeTemplate(tmp *template.Template, env any) []byte {
	// gen src with env
	buff := bytes.NewBufferString("")
	err := tmp.Execute(buff, env)
	if err != nil {
		panic(err)
	}
	//格式化
	source, err := format.Source(buff.Bytes())
	if err != nil {
		panic(err)
	}
	return source
}
