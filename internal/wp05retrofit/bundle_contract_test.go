package wp05retrofit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const duramindRepoEnv = "DURAMIND_REPO"

var contractTypeNames = []string{"Bundle", "ItemResult", "Provenance"}

type bundleContract struct {
	Types []contractType `json:"types"`
}

type contractType struct {
	Name   string          `json:"name"`
	Fields []contractField `json:"fields"`
}

type contractField struct {
	Name    string `json:"name"`
	GoType  string `json:"go_type"`
	JSONTag string `json:"json_tag"`
}

func TestBundleJSONSchemaContract(t *testing.T) {
	golden := readBundleContract(t, filepath.Join("testdata", "duramind_bundle_schema.golden.json"))

	t.Run("engram-go mirror matches golden", func(t *testing.T) {
		actual, err := extractBundleContract("runner.go")
		if err != nil {
			t.Fatalf("extract engram-go bundle contract: %v", err)
		}
		assertBundleContract(t, golden, actual)
	})

	t.Run("duramind authority matches golden", func(t *testing.T) {
		duramindRepo := strings.TrimSpace(os.Getenv(duramindRepoEnv))
		if duramindRepo == "" {
			t.Skipf("%s is unset; CI sets it to the checked-out duramind repository", duramindRepoEnv)
		}

		sourcePath := filepath.Join(duramindRepo, "internal", "wp05c", "compare.go")
		actual, err := extractBundleContract(sourcePath)
		if err != nil {
			t.Fatalf("extract duramind bundle contract: %v", err)
		}
		assertBundleContract(t, golden, actual)
	})
}

func TestBundleContractComparisonRejectsDrift(t *testing.T) {
	contract := bundleContract{
		Types: []contractType{{
			Name: "Bundle",
			Fields: []contractField{{
				Name:    "System",
				GoType:  "string",
				JSONTag: "system",
			}},
		}},
	}
	tests := []struct {
		name    string
		drifted bundleContract
	}{
		{
			name: "changed JSON tag",
			drifted: bundleContract{
				Types: []contractType{{
					Name: "Bundle",
					Fields: []contractField{{
						Name:    "System",
						GoType:  "string",
						JSONTag: "system_name",
					}},
				}},
			},
		},
		{
			name: "added field",
			drifted: bundleContract{
				Types: []contractType{{
					Name: "Bundle",
					Fields: []contractField{
						{
							Name:    "System",
							GoType:  "string",
							JSONTag: "system",
						},
						{
							Name:    "SchemaVersion",
							GoType:  "string",
							JSONTag: "schema_version",
						},
					},
				}},
			},
		},
		{
			name: "removed type",
			drifted: bundleContract{
				Types: []contractType{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := bundleContractDiff(contract, tt.drifted); diff == "" {
				t.Fatal("contract comparison accepted schema drift")
			}
		})
	}
}

func TestExtractBundleContractRejectsMissingType(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "incomplete.go")
	source := []byte(`package incomplete

type Bundle struct {
	System string ` + "`json:\"system\"`" + `
}
`)
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatalf("write incomplete source: %v", err)
	}

	_, err := extractBundleContract(sourcePath)
	if err == nil || !strings.Contains(err.Error(), "ItemResult is missing") {
		t.Fatalf("extractBundleContract error = %v, want missing ItemResult error", err)
	}
}

func TestExtractBundleContractRejectsJSONTagOptionDrift(t *testing.T) {
	tempDir := t.TempDir()
	sourceTemplate := `package contract

type Bundle struct {
	System string ` + "`json:\"SYSTEM_TAG\"`" + `
}

type ItemResult struct {
	QuestionID string ` + "`json:\"question_id\"`" + `
}

type Provenance struct {
	RunID string ` + "`json:\"run_id\"`" + `
}
`
	writeSource := func(name, systemTag string) string {
		t.Helper()
		path := filepath.Join(tempDir, name)
		source := strings.ReplaceAll(sourceTemplate, "SYSTEM_TAG", systemTag)
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	baseline, err := extractBundleContract(writeSource("baseline.go", "system"))
	if err != nil {
		t.Fatalf("extract baseline contract: %v", err)
	}
	drifted, err := extractBundleContract(writeSource("drifted.go", "system,string"))
	if err != nil {
		t.Fatalf("extract drifted contract: %v", err)
	}
	if diff := bundleContractDiff(baseline, drifted); diff == "" {
		t.Fatal("contract comparison accepted a changed JSON tag option")
	}
}

func readBundleContract(t *testing.T, path string) bundleContract {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle contract %s: %v", path, err)
	}

	var contract bundleContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("parse bundle contract %s: %v", path, err)
	}
	return contract
}

func extractBundleContract(path string) (bundleContract, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return bundleContract{}, fmt.Errorf("parse %s: %w", path, err)
	}

	declarations := make(map[string]*ast.StructType, len(contractTypeNames))
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if ok {
				declarations[typeSpec.Name.Name] = structType
			}
		}
	}

	contract := bundleContract{Types: make([]contractType, 0, len(contractTypeNames))}
	for _, typeName := range contractTypeNames {
		structType, ok := declarations[typeName]
		if !ok {
			return bundleContract{}, fmt.Errorf("%s: type %s is missing or is not a struct", path, typeName)
		}
		contractType, err := extractContractType(fset, typeName, structType)
		if err != nil {
			return bundleContract{}, fmt.Errorf("%s: %w", path, err)
		}
		contract.Types = append(contract.Types, contractType)
	}
	return contract, nil
}

func extractContractType(
	fset *token.FileSet,
	typeName string,
	structType *ast.StructType,
) (contractType, error) {
	result := contractType{
		Name:   typeName,
		Fields: make([]contractField, 0, len(structType.Fields.List)),
	}
	for _, field := range structType.Fields.List {
		if len(field.Names) != 1 {
			return contractType{}, fmt.Errorf("type %s has an embedded or multi-name field", typeName)
		}
		if field.Tag == nil {
			return contractType{}, fmt.Errorf("type %s field %s has no JSON tag", typeName, field.Names[0].Name)
		}

		rawTag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			return contractType{}, fmt.Errorf("type %s field %s has invalid tag: %w", typeName, field.Names[0].Name, err)
		}
		jsonTag := reflect.StructTag(rawTag).Get("json")
		jsonParts := strings.Split(jsonTag, ",")
		if jsonParts[0] == "" || jsonParts[0] == "-" {
			return contractType{}, fmt.Errorf("type %s field %s has invalid JSON name %q", typeName, field.Names[0].Name, jsonParts[0])
		}

		var goType bytes.Buffer
		if err := format.Node(&goType, fset, field.Type); err != nil {
			return contractType{}, fmt.Errorf("format type %s field %s: %w", typeName, field.Names[0].Name, err)
		}
		result.Fields = append(result.Fields, contractField{
			Name:    field.Names[0].Name,
			GoType:  goType.String(),
			JSONTag: jsonTag,
		})
	}
	return result, nil
}

func assertBundleContract(t *testing.T, want, got bundleContract) {
	t.Helper()
	if diff := bundleContractDiff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func bundleContractDiff(want, got bundleContract) string {
	if reflect.DeepEqual(want, got) {
		return ""
	}

	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	gotJSON, _ := json.MarshalIndent(got, "", "  ")
	return fmt.Sprintf("bundle JSON schema drifted (-want +got):\nwant:\n%s\ngot:\n%s", wantJSON, gotJSON)
}
