package visitor

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"html/template"
	"os"
	"regexp"
	"slices"
	"strings"
)

func GenByStructComment(inputFile, outputFile, keyword, temp string, scanMethods []string) {
	visitor := Scan(inputFile, keyword, scanMethods)
	fs, err := template.New("").Parse(temp)
	if err != nil {
		panic(err)
	}
	buff := bytes.NewBufferString("")
	err = fs.Execute(buff, visitor)
	if err != nil {
		panic(err)
	}
	//格式化
	src, err := format.Source(buff.Bytes())
	if err != nil {
		panic(err)
	}
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

// Scan 扫描go文件，目标为出现了{keyword}注释的struct，并且只扫描目标方法集scanMethods
func Scan(inputFile, keyword string, scanMethods []string) *StructVisitor {
	fileSet := token.NewFileSet()
	f, err := parser.ParseFile(fileSet, inputFile, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}
	visitor := StructVisitor{
		keyword:     keyword,
		scanMethods: scanMethods,
		fileSet:     fileSet,
		Targets:     make([]TargetStruct, 0),
	}
	ast.Walk(&visitor, f)
	visitor.setDependImports()
	return &visitor
}

type StructVisitor struct {
	keyword       string
	scanMethods   []string
	fileSet       *token.FileSet
	Pkg           string
	Imports       []ImportInfo
	Targets       []TargetStruct
	DependImports []ImportInfo
}

type TargetStruct struct {
	Name    string
	Methods []MethodSignature
}

type MethodSignature struct {
	Name        string
	Params      []ParamSignature
	Results     []ParamSignature
	DependFlags []string
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
	imports := make([]ImportInfo, 0)
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
				imports = append(imports, importInfo)
			}
			receiver.Imports = imports
		} else if genDecl.Tok == token.TYPE {
			// type
			hasKeywordComment := false
			if genDecl.Doc != nil && genDecl.Doc.List != nil {
				// 遍历注释
				for _, comment := range genDecl.Doc.List {
					if strings.Contains(comment.Text, receiver.keyword) {
						hasKeywordComment = true
					}
				}
			}
			if hasKeywordComment {
				specs := genDecl.Specs
				for _, spec := range specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						// type
						name := typeSpec.Name.Name
						proxyTarget := TargetStruct{
							Name: name,
						}
						// collect target methods sign
						methodSigns := make([]MethodSignature, 0)
						if interfaceType, ok := typeSpec.Type.(*ast.InterfaceType); ok {
							methods := interfaceType.Methods
							for _, method := range methods.List {
								if len(method.Names) == 1 {
									methodName := method.Names[0].Name
									if slices.Contains(receiver.scanMethods, methodName) {
										if funcType, ok := method.Type.(*ast.FuncType); ok {
											methodSign := parserMethod(funcType)
											methodSign.Name = method.Names[0].Name
											methodSigns = append(methodSigns, *methodSign)
										}
									}
								}
							}
						}
						proxyTarget.Methods = methodSigns
						receiver.Targets = append(receiver.Targets, proxyTarget)
					}
				}
			}
		}
	}
	// get targets imports
	return receiver
}

func (receiver *StructVisitor) setDependImports() {
	depends := make([]string, 0)
	for _, target := range receiver.Targets {
		for _, method := range target.Methods {
			depends = append(depends, method.DependFlags...)
		}
	}
	dependImports := make([]ImportInfo, 0)
	for _, imp := range receiver.Imports {
		var dependName string
		if imp.Alias != "" {
			dependName = imp.Alias
		} else {
			split := strings.Split(imp.Pkg, "/")
			dependName = split[len(split)-1]
		}
		if slices.Contains(depends, dependName) {
			dependImports = append(dependImports, imp)
		}
	}
	receiver.DependImports = dependImports
}

func parserMethod(funcType *ast.FuncType) *MethodSignature {
	dependFlags := make([]string, 0)
	params := funcType.Params
	paramSignatures := make([]ParamSignature, len(params.List))
	for i, param := range params.List {
		paramSign, depend := parserParam(param)
		paramSignatures[i] = *paramSign
		if depend != nil {
			dependFlags = append(dependFlags, *depend)
		}
	}

	results := funcType.Results
	resultSignatures := make([]ParamSignature, len(results.List))
	for i, result := range results.List {
		paramSign, depend := parserParam(result)
		resultSignatures[i] = *paramSign
		if depend != nil {
			dependFlags = append(dependFlags, *depend)
		}
	}
	return &MethodSignature{
		Params:      paramSignatures,
		Results:     resultSignatures,
		DependFlags: dependFlags,
	}
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
	default:
		panic(fmt.Sprintf("unknown param type:%v", paramType))
	}
	return
}
