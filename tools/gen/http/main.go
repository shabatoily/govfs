package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Schema struct {
	Type       string            `json:"type"`
	Properties map[string]Schema `json:"properties"`
	Ref        string            `json:"$ref"`
	Items      *Schema           `json:"items"`
	OneOf      []Schema          `json:"oneOf"`
	AllOf      []Schema          `json:"allOf"`
}

type MediaType struct {
	Schema Schema `json:"schema"`
}

type RequestBody struct {
	Content  map[string]MediaType `json:"content"`
	Required bool                 `json:"required"`
}

type Operation struct {
	Summary     string       `json:"summary"`
	Description string       `json:"description"`
	RequestBody *RequestBody `json:"requestBody"`
}

type Components struct {
	Schemas map[string]Schema `json:"schemas"`
}

type Swagger struct {
	Paths      map[string]map[string]Operation `json:"paths"`
	Components Components                      `json:"components"`
}

func generateDummy(schema Schema, components Components) any {
	if schema.Ref != "" {
		refName := strings.TrimPrefix(schema.Ref, "#/components/schemas/")
		if resolved, ok := components.Schemas[refName]; ok {
			return generateDummy(resolved, components)
		}
		return map[string]any{}
	}

	if len(schema.AllOf) > 0 {
		res := map[string]any{}
		for _, sub := range schema.AllOf {
			if subRes, ok := generateDummy(sub, components).(map[string]any); ok {
				for k, v := range subRes {
					res[k] = v
				}
			}
		}
		return res
	}

	if len(schema.OneOf) > 0 {
		return generateDummy(schema.OneOf[len(schema.OneOf)-1], components)
	}

	switch schema.Type {
	case "string":
		return "string"
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "array":
		if schema.Items != nil {
			return []any{generateDummy(*schema.Items, components)}
		}
		return []any{}
	default:
		res := map[string]any{}
		for name, prop := range schema.Properties {
			res[name] = generateDummy(prop, components)
		}
		return res
	}
}

func main() {
	inputPath := flag.String("in", "docs/swagger.json", "Path to swagger.json input file")
	outputPath := flag.String("out", "docs/api.http", "Path to api.http output file")
	flag.Parse()

	data, err := os.ReadFile(*inputPath)
	if err != nil {
		fmt.Printf("Error reading input file: %v\n", err)
		os.Exit(1)
	}

	var swagger Swagger
	if err := json.Unmarshal(data, &swagger); err != nil {
		fmt.Printf("Error parsing swagger JSON: %v\n", err)
		os.Exit(1)
	}

	var lines []string

	var paths []string
	for path := range swagger.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		methods := swagger.Paths[path]

		var methodNames []string
		for method := range methods {
			methodNames = append(methodNames, method)
		}
		sort.Strings(methodNames)

		for _, method := range methodNames {
			details := methods[method]
			summary := details.Summary
			if summary == "" {
				summary = "No summary"
			}
			description := details.Description
			if description == "" {
				description = "No description"
			}

			summary = strings.ReplaceAll(summary, "\n", " ")
			description = strings.ReplaceAll(description, "\n", " ")
			url := fmt.Sprintf("http://localhost:3000%s", path)

			lines = append(lines, fmt.Sprintf("### %s %s", summary, description))

			// Handle request bodies
			hasBody := false
			bodyStr := ""
			headers := []string{}

			if details.RequestBody != nil && len(details.RequestBody.Content) > 0 {
				hasBody = true
				if mt, ok := details.RequestBody.Content["application/json"]; ok {
					headers = append(headers, "Content-Type: application/json")
					dummy := generateDummy(mt.Schema, swagger.Components)
					b, _ := json.MarshalIndent(dummy, "", "  ")
					bodyStr = string(b)
				} else if _, ok := details.RequestBody.Content["multipart/form-data"]; ok {
					headers = append(headers, "Content-Type: multipart/form-data; boundary=WebAppBoundary")
					bodyStr = "--WebAppBoundary\nContent-Disposition: form-data; name=\"file\"; filename=\"test.txt\"\nContent-Type: text/plain\n\nHello World\n--WebAppBoundary--"
				} else {
					// Fallback
					headers = append(headers, "Content-Type: application/json")
					bodyStr = "{}"
				}
			}

			lines = append(lines, fmt.Sprintf("%s %s HTTP/1.1", strings.ToUpper(method), url))
			for _, h := range headers {
				lines = append(lines, h)
			}

			if hasBody {
				lines = append(lines, "")
				lines = append(lines, bodyStr)
			}
			lines = append(lines, "")
		}
	}

	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	outputContent := strings.Join(lines, "\n")
	if err := os.WriteFile(*outputPath, []byte(outputContent), 0o644); err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s from %s\n", *outputPath, *inputPath)
}
