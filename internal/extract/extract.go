package extract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func Parse(file model.File) ([]model.Symbol, []model.Edge, error) {
	if strings.HasSuffix(file.Path, "package.json") {
		return nil, packageEdges(file), nil
	}
	if file.Classification == "configuration" || file.Language == "yaml" || file.Language == "yml" || file.Language == "properties" {
		return nil, configurationEdges(file), nil
	}
	var lang *treesitter.Language
	switch file.Language {
	case "typescript":
		lang = treesitter.NewLanguage(typescript.LanguageTypescript())
	case "tsx":
		lang = treesitter.NewLanguage(typescript.LanguageTSX())
	case "javascript", "jsx":
		lang = treesitter.NewLanguage(javascript.Language())
	case "java":
		lang = treesitter.NewLanguage(java.Language())
	case "kotlin":
		lang = treesitter.NewLanguage(kotlin.Language())
	default:
		return nil, nil, nil
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(lang); err != nil {
		return nil, nil, fmt.Errorf("set %s language: %w", file.Language, err)
	}
	source := []byte(file.Content)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("parse %s: no syntax tree", file.Path)
	}
	defer tree.Close()
	var symbols []model.Symbol
	var edges []model.Edge
	walk(tree.RootNode(), func(node *treesitter.Node) {
		if kind, ok := declarationKind(file.Language, node.Kind()); ok {
			if nameNode := node.ChildByFieldName("name"); nameNode != nil {
				name := nameNode.Utf8Text(source)
				if name != "" {
					symbols = append(symbols, model.Symbol{
						RepoID: file.RepoID, Path: file.Path, Name: name, Kind: kind,
						Span: span(node), Extractor: model.ExtractorVersion, Confidence: 1,
					})
				}
			}
		}
		switch {
		case isJSLanguage(file.Language) && node.Kind() == "import_statement":
			if target := stringValue(node.ChildByFieldName("source"), source); target != "" {
				edges = append(edges, edge(file, nearestSymbol(node, source), target, "IMPORTS", node, "tree-sitter-import", 1))
			}
		case file.Language == "java" && node.Kind() == "import_declaration":
			target := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(node.Utf8Text(source)), "import")), ";")
			target = strings.TrimSpace(strings.TrimPrefix(target, "static"))
			if target != "" {
				edges = append(edges, edge(file, nearestSymbol(node, source), target, "IMPORTS", node, "tree-sitter-import", 1))
			}
		case file.Language == "kotlin" && node.Kind() == "import":
			target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(node.Utf8Text(source)), "import"))
			if target != "" {
				edges = append(edges, edge(file, nearestSymbol(node, source), target, "IMPORTS", node, "tree-sitter-kotlin-import", 1))
			}
		case isJSXLanguage(file.Language) && (node.Kind() == "jsx_opening_element" || node.Kind() == "jsx_self_closing_element"):
			if target := reactRoute(node, source); target != "" {
				edges = append(edges, edge(file, nearestSymbol(node, source), target, "DECLARES_UI_ROUTE", node, "tree-sitter-react-route", .95))
				if component := reactRouteComponent(node, source); component != "" {
					edges = append(edges, edge(file, target, component, "ROUTE_RENDERS_COMPONENT", node, "tree-sitter-react-route", .95))
				}
			}
		case isJSLanguage(file.Language) && node.Kind() == "call_expression":
			fn := node.ChildByFieldName("function")
			name := callName(fn, source)
			if name == "registerApplication" {
				if target := firstString(node, source); target != "" {
					edges = append(edges, edge(file, nearestSymbol(node, source), target, "REGISTERS_MICROFRONTEND", node, "tree-sitter-single-spa-call", .85))
				}
			}
			if fn != nil {
				if name == "fetch" || name == "axios.get" || name == "axios.post" || name == "axios.put" || name == "axios.patch" || name == "axios.delete" {
					if target := firstString(node, source); target != "" {
						edges = append(edges, withLocalGuard(file, node, source, edge(file, nearestSymbol(node, source), target, "INVOKES_API", node, "tree-sitter-ui-api-call", .95))...)
						method := "GET"
						if name != "fetch" {
							method = strings.ToUpper(strings.TrimPrefix(name, "axios."))
						}
						edges = append(edges, withLocalGuard(file, node, source, edge(file, nearestSymbol(node, source), method+" "+target, "INVOKES_API", node, "tree-sitter-ui-api-method", .95))...)
					} else {
						edges = append(edges, coverageGap(file, nearestSymbol(node, source), "nonliteral_http_url", node, "tree-sitter-ui-api-call"))
					}
				} else if name == "console.error" || name == "console.warn" || name == "console.log" || name == "console.info" {
					kind := "EMITS_LOG"
					if name == "console.error" {
						kind = "EMITS_ERROR"
					}
					if target := firstString(node, source); target != "" {
						edges = append(edges, withLocalGuard(file, node, source, edge(file, nearestSymbol(node, source), target, kind, node, "tree-sitter-typescript-log", .95))...)
					} else {
						edges = append(edges, coverageGap(file, nearestSymbol(node, source), "nonliteral_log_message", node, "tree-sitter-typescript-log"))
					}
				} else if strings.HasSuffix(name, ".dispatchEvent") || strings.HasSuffix(name, ".addEventListener") || name == "dispatchEvent" || name == "addEventListener" {
					if target := firstString(node, source); target != "" {
						predicate := "SUBSCRIBES_EVENT"
						if strings.HasSuffix(name, "dispatchEvent") {
							predicate = "PUBLISHES_EVENT"
						}
						edges = append(edges, withLocalGuard(file, node, source, edge(file, nearestSymbol(node, source), target, predicate, node, "tree-sitter-browser-event", .95))...)
					} else {
						edges = append(edges, coverageGap(file, nearestSymbol(node, source), "dynamic_browser_event", node, "tree-sitter-browser-event"))
					}
				} else if isDirectIdentifier(name) {
					edges = append(edges, edge(file, nearestSymbol(node, source), name, "CALLS", node, "tree-sitter-call", .8))
				}
			}
		case isJSLanguage(file.Language) && node.Kind() == "member_expression":
			if config := typescriptConfigReference(node, source); config != "" {
				edges = append(edges, edge(file, nearestSymbol(node, source), config, "REFERENCES_CONFIGURATION", node, "tree-sitter-typescript-config", .95))
			}
		case isJSLanguage(file.Language) && node.Kind() == "throw_statement":
			edges = append(edges, withLocalGuard(file, node, source, typescriptThrowEdges(file, node, source)...)...)
		case file.Language == "java" && (node.Kind() == "annotation" || node.Kind() == "marker_annotation"):
			edges = append(edges, withLocalGuard(file, node, source, javaAnnotationEdges(file, node, source)...)...)
		case file.Language == "java" && node.Kind() == "method_invocation":
			edges = append(edges, withLocalGuard(file, node, source, javaCallEdges(file, node, source)...)...)
		case file.Language == "java" && node.Kind() == "throw_statement":
			edges = append(edges, withLocalGuard(file, node, source, javaThrowEdges(file, node, source)...)...)
		case file.Language == "kotlin" && node.Kind() == "annotation":
			edges = append(edges, withLocalGuard(file, node, source, kotlinAnnotationEdges(file, node, source)...)...)
		case file.Language == "kotlin" && node.Kind() == "call_expression":
			edges = append(edges, withLocalGuard(file, node, source, kotlinCallEdges(file, node, source)...)...)
		}
	})
	if file.Language == "java" {
		edges = append(edges, effectiveJavaRoutes(file, tree.RootNode(), source)...)
	}
	return symbols, edges, nil
}

func packageEdges(file model.File) []model.Edge {
	var doc struct {
		Name            string            `json:"name"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal([]byte(file.Content), &doc) != nil {
		return nil
	}
	var out []model.Edge
	add := func(name, kind string) {
		if name == "" {
			return
		}
		start := strings.Index(file.Content, "\""+name+"\"")
		if start < 0 {
			return
		}
		out = append(out, model.Edge{RepoID: file.RepoID, Path: file.Path, Source: "<file>", Target: name, Kind: kind, Span: byteSpanForContent(file.Content, start+1, start+1+len(name)), Resolver: "package-json", Derivation: "syntax_derived", Confidence: .95})
	}
	add(doc.Name, "DECLARES_MODULE")
	for n := range doc.Dependencies {
		add(n, "DEPENDENCY_DECLARATION")
	}
	for n := range doc.DevDependencies {
		add(n, "DEPENDENCY_DECLARATION")
	}
	return out
}

func configurationEdges(file model.File) []model.Edge {
	var edges []model.Edge
	offset := 0
	for _, line := range strings.SplitAfter(file.Content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			offset += len(line)
			continue
		}
		separator := strings.IndexAny(trimmed, ":=")
		if separator > 0 {
			key := strings.TrimSpace(trimmed[:separator])
			if isConfigurationKey(key) {
				start := offset + strings.Index(line, key)
				edges = append(edges, model.Edge{RepoID: file.RepoID, Path: file.Path, Source: "<file>", Target: key, Kind: "DEFINES_CONFIGURATION", Span: byteSpanForContent(file.Content, start, start+len(key)), Resolver: "line-config-key", Confidence: .95})
			}
		}
		for _, reference := range configurationReferences(trimmed) {
			start := offset + strings.Index(line, reference)
			edges = append(edges, model.Edge{RepoID: file.RepoID, Path: file.Path, Source: "<file>", Target: reference, Kind: "REFERENCES_CONFIGURATION", Span: byteSpanForContent(file.Content, start, start+len(reference)), Resolver: "config-placeholder-reference", Confidence: .95})
		}
		offset += len(line)
	}
	return edges
}

func isDirectIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func isConfigurationKey(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r == '_' || r == '-' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func byteSpanForContent(content string, start, end int) model.Span {
	prefix := content[:start]
	endPrefix := content[:end]
	lineStart := strings.LastIndex(prefix, "\n") + 1
	endLineStart := strings.LastIndex(endPrefix, "\n") + 1
	return model.Span{StartByte: start, EndByte: end, StartLine: strings.Count(prefix, "\n") + 1, StartColumn: start - lineStart + 1, EndLine: strings.Count(endPrefix, "\n") + 1, EndColumn: end - endLineStart + 1}
}

var publishedEventPattern = regexp.MustCompile(`(?:publishEvent|multicastEvent)\s*\(\s*new\s+([A-Za-z_$][A-Za-z0-9_$.]*)`)
var configReferencePattern = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)(?::[^}]*)?\}`)
var sqlTablePattern = regexp.MustCompile(`(?i)^\s*(?:select\s+.+?\s+from|delete\s+from|insert\s+into|update)\s+([A-Za-z_][A-Za-z0-9_$.]*)`)
var classLiteralPattern = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$.]*)\.class\b`)
var reactRoutePattern = regexp.MustCompile(`^<Route\b[^>]*\bpath\s*=\s*["']([^"']+)["']`)
var reactRouteComponentPattern = regexp.MustCompile(`\belement\s*=\s*\{\s*<([A-Za-z_$][A-Za-z0-9_$]*)`)
var tsConfigPattern = regexp.MustCompile(`(?:process\.env|import\.meta\.env)\.([A-Za-z_][A-Za-z0-9_]*)`)
var kotlinAnnotationPattern = regexp.MustCompile(`@(?:[A-Za-z_][A-Za-z0-9_]*:)?([A-Za-z_][A-Za-z0-9_.]*)`)
var kotlinCallPattern = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$]*(?:\??\.[A-Za-z_$][A-Za-z0-9_$]*)*)\s*\(`)

func publishedEvent(text string) string {
	match := publishedEventPattern.FindStringSubmatch(text)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func declarationKind(language, nodeKind string) (string, bool) {
	if language == "java" {
		kinds := map[string]string{
			"class_declaration": "class", "interface_declaration": "interface",
			"enum_declaration": "enum", "record_declaration": "record",
			"annotation_type_declaration": "annotation", "variable_declarator": "variable",
			"method_declaration": "method", "constructor_declaration": "constructor",
		}
		kind, ok := kinds[nodeKind]
		return kind, ok
	}
	if language == "kotlin" {
		kinds := map[string]string{
			"class_declaration": "class", "object_declaration": "object",
			"function_declaration": "function", "property_declaration": "property",
		}
		kind, ok := kinds[nodeKind]
		return kind, ok
	}
	kinds := map[string]string{
		"class_declaration": "class", "interface_declaration": "interface",
		"enum_declaration": "enum", "type_alias_declaration": "type",
		"function_declaration": "function", "method_definition": "method",
		"method_signature": "method", "variable_declarator": "variable",
	}
	kind, ok := kinds[nodeKind]
	return kind, ok
}

func isJSLanguage(language string) bool {
	return language == "typescript" || language == "tsx" || language == "javascript" || language == "jsx"
}

func isJSXLanguage(language string) bool { return language == "tsx" || language == "jsx" }

func javaAnnotationEdges(file model.File, node *treesitter.Node, source []byte) []model.Edge {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := simpleName(nameNode.Utf8Text(source))
	target := annotationTarget(node, source)
	if target == "" {
		switch name {
		case "RequestMapping", "GetMapping", "PostMapping", "PutMapping", "PatchMapping", "DeleteMapping", "JmsListener", "KafkaListener", "Value", "ConfigurationProperties", "Table":
			return []model.Edge{coverageGap(file, nearestSymbol(node, source), "dynamic_"+strings.ToLower(name), node, "tree-sitter-java-annotation")}
		}
		return nil
	}
	switch name {
	case "RequestMapping", "GetMapping", "PostMapping", "PutMapping", "PatchMapping", "DeleteMapping":
		return []model.Edge{edge(file, nearestSymbol(node, source), target, "DECLARES_ENDPOINT", node, "tree-sitter-spring-route-annotation", .9)}
	case "JmsListener":
		return []model.Edge{edge(file, nearestSymbol(node, source), target, "LISTENS_QUEUE", node, "tree-sitter-jms-listener", .95), edge(file, nearestSymbol(node, source), target, "CONSUMES_EVENT", node, "tree-sitter-jms-listener", .9)}
	case "KafkaListener":
		return []model.Edge{edge(file, nearestSymbol(node, source), target, "CONSUMES_TOPIC", node, "tree-sitter-kafka-listener", .95), edge(file, nearestSymbol(node, source), target, "CONSUMES_EVENT", node, "tree-sitter-kafka-listener", .9)}
	case "EventListener":
		return []model.Edge{edge(file, nearestSymbol(node, source), target, "LISTENS_EVENT", node, "tree-sitter-spring-event-listener", .95), edge(file, nearestSymbol(node, source), target, "CONSUMES_EVENT", node, "tree-sitter-spring-event-listener", .9)}
	case "FeignClient":
		return []model.Edge{edge(file, nearestSymbol(node, source), target, "DEPENDS_ON_EXTERNAL", node, "tree-sitter-feign-client", .95)}
	case "Value":
		for _, ref := range configurationReferences(target) {
			return []model.Edge{edge(file, nearestSymbol(node, source), ref, "REFERENCES_CONFIGURATION", node, "tree-sitter-spring-value", .95)}
		}
	case "ConfigurationProperties":
		return []model.Edge{edge(file, nearestSymbol(node, source), target, "BINDS_CONFIGURATION", node, "tree-sitter-configuration-properties", .95)}
	case "Table":
		return []model.Edge{edge(file, nearestSymbol(node, source), target, "MAPS_TABLE", node, "tree-sitter-jpa-table", .95)}
	}
	return nil
}

func kotlinAnnotationEdges(file model.File, node *treesitter.Node, source []byte) []model.Edge {
	text := node.Utf8Text(source)
	match := kotlinAnnotationPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return nil
	}
	name := simpleName(match[1])
	target := firstString(node, source)
	from := nearestSymbol(node, source)
	if target == "" {
		switch name {
		case "RequestMapping", "GetMapping", "PostMapping", "PutMapping", "PatchMapping", "DeleteMapping", "JmsListener", "KafkaListener", "Value", "ConfigurationProperties", "Table":
			return []model.Edge{coverageGap(file, from, "dynamic_"+strings.ToLower(name), node, "tree-sitter-kotlin-annotation")}
		}
		return nil
	}
	switch name {
	case "RequestMapping", "GetMapping", "PostMapping", "PutMapping", "PatchMapping", "DeleteMapping":
		return []model.Edge{edge(file, from, target, "DECLARES_ENDPOINT", node, "tree-sitter-kotlin-spring-route", .9)}
	case "JmsListener":
		return []model.Edge{edge(file, from, target, "LISTENS_QUEUE", node, "tree-sitter-kotlin-jms-listener", .95), edge(file, from, target, "CONSUMES_EVENT", node, "tree-sitter-kotlin-jms-listener", .9)}
	case "KafkaListener":
		return []model.Edge{edge(file, from, target, "CONSUMES_TOPIC", node, "tree-sitter-kotlin-kafka-listener", .95), edge(file, from, target, "CONSUMES_EVENT", node, "tree-sitter-kotlin-kafka-listener", .9)}
	case "EventListener":
		return []model.Edge{edge(file, from, target, "LISTENS_EVENT", node, "tree-sitter-kotlin-event-listener", .95), edge(file, from, target, "CONSUMES_EVENT", node, "tree-sitter-kotlin-event-listener", .9)}
	case "FeignClient":
		return []model.Edge{edge(file, from, target, "DEPENDS_ON_EXTERNAL", node, "tree-sitter-kotlin-feign-client", .95)}
	case "Value":
		for _, reference := range configurationReferences(target) {
			return []model.Edge{edge(file, from, reference, "REFERENCES_CONFIGURATION", node, "tree-sitter-kotlin-spring-value", .95)}
		}
	case "ConfigurationProperties":
		return []model.Edge{edge(file, from, target, "BINDS_CONFIGURATION", node, "tree-sitter-kotlin-configuration-properties", .95)}
	case "Table":
		return []model.Edge{edge(file, from, target, "MAPS_TABLE", node, "tree-sitter-kotlin-jpa-table", .95)}
	}
	return nil
}

func kotlinCallEdges(file model.File, node *treesitter.Node, source []byte) []model.Edge {
	text := strings.TrimSpace(node.Utf8Text(source))
	match := kotlinCallPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return nil
	}
	qualified := strings.ReplaceAll(match[1], "?.", ".")
	parts := strings.Split(qualified, ".")
	name, object := parts[len(parts)-1], ""
	if len(parts) > 1 {
		object = parts[len(parts)-2]
	}
	target, from := firstString(node, source), nearestSymbol(node, source)
	if (name == "convertAndSend" || name == "send") && (object == "jmsTemplate" || object == "kafkaTemplate") {
		if target == "" {
			return []model.Edge{coverageGap(file, from, "computed_message_destination", node, "tree-sitter-kotlin-message-send")}
		}
		if object == "kafkaTemplate" {
			return []model.Edge{edge(file, from, target, "PRODUCES_TOPIC", node, "tree-sitter-kotlin-kafka-send", .95), edge(file, from, target, "EMITS_EVENT", node, "tree-sitter-kotlin-kafka-send", .9)}
		}
		return []model.Edge{edge(file, from, target, "SENDS_QUEUE", node, "tree-sitter-kotlin-jms-send", .95), edge(file, from, target, "EMITS_EVENT", node, "tree-sitter-kotlin-jms-send", .9)}
	}
	if name == "publishEvent" {
		if event := publishedEvent(text); event != "" {
			return []model.Edge{edge(file, from, event, "PUBLISHES_EVENT", node, "tree-sitter-kotlin-event-publication", .95), edge(file, from, event, "EMITS_EVENT", node, "tree-sitter-kotlin-event-publication", .82)}
		}
	}
	if target != "" && (object == "restTemplate" || object == "webClient" || object == "restClient") {
		return []model.Edge{edge(file, from, target, "INVOKES_API", node, "tree-sitter-kotlin-http-client", .95)}
	}
	if name == "error" || name == "warn" || name == "info" || name == "debug" {
		if target == "" {
			return []model.Edge{coverageGap(file, from, "nonliteral_log_message", node, "tree-sitter-kotlin-log")}
		}
		kind := "EMITS_LOG"
		if name == "error" {
			kind = "EMITS_ERROR"
		}
		return []model.Edge{edge(file, from, target, kind, node, "tree-sitter-kotlin-log", .95)}
	}
	if object == "" && isDirectIdentifier(name) {
		return []model.Edge{edge(file, from, name, "CALLS", node, "tree-sitter-kotlin-call", .8)}
	}
	return nil
}

func javaCallEdges(file model.File, node *treesitter.Node, source []byte) []model.Edge {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Utf8Text(source)
	object := ""
	if objectNode := node.ChildByFieldName("object"); objectNode != nil {
		object = simpleName(objectNode.Utf8Text(source))
	}
	target := firstString(node, source)
	from := nearestSymbol(node, source)
	if (name == "convertAndSend" || name == "send") && (object == "jmsTemplate" || object == "kafkaTemplate") && target == "" {
		return []model.Edge{coverageGap(file, from, "computed_message_destination", node, "tree-sitter-message-send")}
	}
	if (name == "convertAndSend" || name == "send") && target != "" && (object == "jmsTemplate" || object == "kafkaTemplate") {
		if object == "kafkaTemplate" {
			return []model.Edge{edge(file, from, target, "PRODUCES_TOPIC", node, "tree-sitter-kafka-send", .95), edge(file, from, target, "EMITS_EVENT", node, "tree-sitter-kafka-send", .9)}
		}
		return []model.Edge{edge(file, from, target, "SENDS_QUEUE", node, "tree-sitter-jms-send", .95), edge(file, from, target, "EMITS_EVENT", node, "tree-sitter-jms-send", .9)}
	}
	if name == "publishEvent" {
		if target := publishedEvent(node.Utf8Text(source)); target != "" {
			return []model.Edge{edge(file, from, target, "PUBLISHES_EVENT", node, "tree-sitter-spring-event-publication", .95), edge(file, from, target, "EMITS_EVENT", node, "tree-sitter-spring-event-publication", .82)}
		}
	}
	if target != "" && (object == "restTemplate" || object == "webClient" || object == "restClient") {
		method := ""
		lower := strings.ToLower(name)
		switch {
		case strings.HasPrefix(lower, "get"):
			method = "GET"
		case strings.HasPrefix(lower, "post"):
			method = "POST"
		case strings.HasPrefix(lower, "put"):
			method = "PUT"
		case strings.HasPrefix(lower, "delete"):
			method = "DELETE"
		case strings.HasPrefix(lower, "patch"):
			method = "PATCH"
		}
		out := []model.Edge{edge(file, from, target, "INVOKES_API", node, "tree-sitter-java-http-call", .9)}
		if method != "" {
			out = append(out, edge(file, from, method+" "+target, "INVOKES_API", node, "tree-sitter-java-http-method", .9))
		}
		return out
	}
	if (object == "log" || object == "logger") && (name == "trace" || name == "debug" || name == "info" || name == "warn" || name == "error") {
		kind := "EMITS_LOG"
		if name == "error" {
			kind = "EMITS_ERROR"
		}
		if target != "" {
			return []model.Edge{edge(file, from, target, kind, node, "tree-sitter-java-log", .95)}
		}
		return []model.Edge{coverageGap(file, from, "nonliteral_log_message", node, "tree-sitter-java-log")}
	}
	if target != "" && (name == "query" || name == "queryForObject" || name == "queryForList" || name == "update" || name == "execute") {
		if table, write := sqlTable(target); table != "" {
			kind := "READS_TABLE"
			if write {
				kind = "WRITES_TABLE"
			}
			return []model.Edge{edge(file, from, table, kind, node, "tree-sitter-sql-call", .9)}
		}
	}
	if isDirectIdentifier(name) {
		return []model.Edge{edge(file, from, name, "CALLS", node, "tree-sitter-call", .8)}
	}
	return nil
}

func javaThrowEdges(file model.File, node *treesitter.Node, source []byte) []model.Edge {
	text := node.Utf8Text(source)
	match := regexp.MustCompile(`throw\s+new\s+([A-Za-z_$][A-Za-z0-9_$.]*)`).FindStringSubmatch(text)
	if len(match) != 2 {
		return []model.Edge{coverageGap(file, nearestSymbol(node, source), "dynamic_error_emission", node, "tree-sitter-java-throw")}
	}
	return []model.Edge{edge(file, nearestSymbol(node, source), match[1], "EMITS_ERROR", node, "tree-sitter-java-throw", .95)}
}

func effectiveJavaRoutes(file model.File, root *treesitter.Node, source []byte) []model.Edge {
	var out []model.Edge
	walk(root, func(node *treesitter.Node) {
		if node.Kind() != "method_declaration" {
			return
		}
		methodRoutes := mappingAnnotations(node, source)
		if len(methodRoutes) == 0 {
			return
		}
		prefix := ""
		for p := node.Parent(); p != nil; p = p.Parent() {
			if p.Kind() == "class_declaration" || p.Kind() == "interface_declaration" {
				prefix = firstMappingPath(p, source)
				break
			}
		}
		for _, route := range methodRoutes {
			path := joinRoute(prefix, route.path)
			if path == "" {
				continue
			}
			out = append(out, edge(file, nearestSymbol(node, source), route.method+" "+path, "DECLARES_EFFECTIVE_ENDPOINT", route.node, "tree-sitter-spring-effective-route", .9))
		}
	})
	return out
}

type javaMapping struct {
	method, path string
	node         *treesitter.Node
}

func mappingAnnotations(node *treesitter.Node, source []byte) []javaMapping {
	var out []javaMapping
	walk(node, func(n *treesitter.Node) {
		if n.Kind() != "annotation" && n.Kind() != "marker_annotation" {
			return
		}
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			return
		}
		name := simpleName(nameNode.Utf8Text(source))
		method := map[string]string{"GetMapping": "GET", "PostMapping": "POST", "PutMapping": "PUT", "PatchMapping": "PATCH", "DeleteMapping": "DELETE"}[name]
		if name == "RequestMapping" {
			method = "REQUEST"
		}
		if method == "" {
			return
		}
		if target := annotationTarget(n, source); target != "" {
			out = append(out, javaMapping{method, target, n})
		}
	})
	return out
}
func firstMappingPath(node *treesitter.Node, source []byte) string {
	x := mappingAnnotations(node, source)
	if len(x) > 0 {
		return x[0].path
	}
	return ""
}
func joinRoute(prefix, suffix string) string {
	if prefix == "" {
		return suffix
	}
	if suffix == "" {
		return prefix
	}
	return "/" + strings.Trim(prefix, "/") + "/" + strings.Trim(suffix, "/")
}

func coverageGap(file model.File, from, code string, node *treesitter.Node, resolver string) model.Edge {
	return model.Edge{RepoID: file.RepoID, Path: file.Path, Source: from, Target: code, Kind: "COVERAGE_GAP", Span: span(node), Resolver: resolver, Derivation: "coverage_gap", Confidence: 0}
}

func withLocalGuard(file model.File, node *treesitter.Node, source []byte, facts ...model.Edge) []model.Edge {
	if len(facts) == 0 {
		return nil
	}
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if parent.Kind() != "if_statement" && parent.Kind() != "conditional_expression" {
			continue
		}
		condition := parent.ChildByFieldName("condition")
		if condition == nil {
			continue
		}
		guard := strings.Trim(strings.TrimSpace(condition.Utf8Text(source)), "()")
		if guard == "" {
			continue
		}
		facts = append(facts, model.Edge{RepoID: file.RepoID, Path: file.Path, Source: nearestSymbol(node, source), Target: guard, Kind: "HAS_LOCAL_GUARD", Span: span(condition), Resolver: "tree-sitter-local-guard", Derivation: "local_syntax", Confidence: 1})
		break
	}
	return facts
}

func callName(node *treesitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.Utf8Text(source))
}
func annotationTarget(node *treesitter.Node, source []byte) string {
	if target := firstString(node, source); target != "" {
		return target
	}
	if match := classLiteralPattern.FindStringSubmatch(node.Utf8Text(source)); len(match) == 2 {
		return match[1]
	}
	return ""
}
func simpleName(value string) string {
	if i := strings.LastIndex(value, "."); i >= 0 {
		return value[i+1:]
	}
	return value
}
func configurationReferences(value string) []string {
	matches := configReferencePattern.FindAllStringSubmatch(value, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match[1])
	}
	return out
}
func sqlTable(statement string) (string, bool) {
	match := sqlTablePattern.FindStringSubmatch(statement)
	if len(match) != 2 {
		return "", false
	}
	lower := strings.ToLower(strings.TrimSpace(statement))
	return match[1], strings.HasPrefix(lower, "insert") || strings.HasPrefix(lower, "update") || strings.HasPrefix(lower, "delete")
}
func reactRoute(node *treesitter.Node, source []byte) string {
	match := reactRoutePattern.FindStringSubmatch(strings.TrimSpace(node.Utf8Text(source)))
	if len(match) == 2 {
		return match[1]
	}
	return ""
}
func reactRouteComponent(node *treesitter.Node, source []byte) string {
	match := reactRouteComponentPattern.FindStringSubmatch(node.Utf8Text(source))
	if len(match) == 2 {
		return match[1]
	}
	return ""
}
func typescriptConfigReference(node *treesitter.Node, source []byte) string {
	match := tsConfigPattern.FindStringSubmatch(node.Utf8Text(source))
	if len(match) == 2 {
		return match[1]
	}
	return ""
}
func typescriptThrowEdges(file model.File, node *treesitter.Node, source []byte) []model.Edge {
	match := regexp.MustCompile(`throw\s+new\s+([A-Za-z_$][A-Za-z0-9_$.]*)`).FindStringSubmatch(node.Utf8Text(source))
	if len(match) != 2 {
		return []model.Edge{coverageGap(file, nearestSymbol(node, source), "dynamic_error_emission", node, "tree-sitter-typescript-throw")}
	}
	return []model.Edge{edge(file, nearestSymbol(node, source), match[1], "EMITS_ERROR", node, "tree-sitter-typescript-throw", .95)}
}

func edge(file model.File, source, target, kind string, node *treesitter.Node, resolver string, confidence float64) model.Edge {
	return model.Edge{RepoID: file.RepoID, Path: file.Path, Source: source, Target: target, Kind: kind, Span: span(node), Resolver: resolver, Confidence: confidence}
}

func nearestSymbol(node *treesitter.Node, source []byte) string {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		for _, language := range []string{"java", "typescript", "kotlin"} {
			if _, ok := declarationKind(language, parent.Kind()); ok {
				if name := parent.ChildByFieldName("name"); name != nil {
					return name.Utf8Text(source)
				}
			}
		}
	}
	return "<file>"
}

func firstString(node *treesitter.Node, source []byte) string {
	var result string
	walk(node, func(child *treesitter.Node) {
		if result != "" {
			return
		}
		if child.Kind() == "string" || child.Kind() == "string_literal" {
			result = stringValue(child, source)
		}
	})
	return result
}

func stringValue(node *treesitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	return strings.Trim(node.Utf8Text(source), "\"'`")
}

func walk(node *treesitter.Node, visit func(*treesitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		walk(node.NamedChild(i), visit)
	}
}

func span(node *treesitter.Node) model.Span {
	start, end := node.StartPosition(), node.EndPosition()
	return model.Span{
		StartByte: int(node.StartByte()), EndByte: int(node.EndByte()),
		StartLine: int(start.Row) + 1, StartColumn: int(start.Column) + 1,
		EndLine: int(end.Row) + 1, EndColumn: int(end.Column) + 1,
	}
}
