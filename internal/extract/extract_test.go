package extract

import (
	"testing"

	"github.com/AdamNi-7080/AIOS/internal/model"
)

func TestTypeScriptAndTSXFacts(t *testing.T) {
	for _, language := range []string{"typescript", "tsx"} {
		t.Run(language, func(t *testing.T) {
			content := `import { start } from "./bootstrap";
export interface AppProps { accountId: string }
export class AccountApp { run() { return start(); } }
export const accountRoute = "/account";
export function mount() { registerApplication("account-homepage", () => import("./app")); }
`
			symbols, edges, err := Parse(model.File{RepoID: "ui", Path: "src/app." + map[string]string{"typescript": "ts", "tsx": "tsx"}[language], Language: language, Content: content})
			if err != nil {
				t.Fatal(err)
			}
			assertSymbol(t, symbols, "AppProps", "interface")
			assertSymbol(t, symbols, "AccountApp", "class")
			assertSymbol(t, symbols, "accountRoute", "variable")
			assertSymbol(t, symbols, "mount", "function")
			assertEdge(t, edges, "IMPORTS", "./bootstrap", 1)
			assertEdge(t, edges, "REGISTERS_MICROFRONTEND", "account-homepage", .85)
		})
	}
}

func TestJavaScriptAndJSXFacts(t *testing.T) {
	content := `import axios from "axios";
export class AccountApp { run() { return axios.get("/api/accounts"); } }
export function App() { return <Route path="/accounts" element={<Accounts/>} />; }
`
	symbols, edges, err := Parse(model.File{RepoID: "ui", Path: "src/app.jsx", Language: "jsx", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	assertSymbol(t, symbols, "AccountApp", "class")
	assertSymbol(t, symbols, "App", "function")
	assertEdge(t, edges, "IMPORTS", "axios", 1)
	assertEdge(t, edges, "INVOKES_API", "/api/accounts", .95)
	assertEdge(t, edges, "DECLARES_UI_ROUTE", "/accounts", .95)
}

func TestKotlinSymbolsSpringContractsAndCalls(t *testing.T) {
	content := `package example
import org.springframework.web.bind.annotation.GetMapping
@FeignClient(name = "ledger")
interface LedgerClient
class AccountController {
  @GetMapping("/accounts")
  fun accounts() = restTemplate.getForObject("https://accounts", String::class.java)
  @KafkaListener(topics = ["account-events"])
  fun consume() = logger.info("received")
  fun publish(event: Any) = kafkaTemplate.send("account-events", event)
}`
	symbols, edges, err := Parse(model.File{RepoID: "account", Path: "AccountController.kt", Language: "kotlin", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	assertSymbol(t, symbols, "LedgerClient", "class")
	assertSymbol(t, symbols, "AccountController", "class")
	assertSymbol(t, symbols, "accounts", "function")
	assertEdge(t, edges, "IMPORTS", "org.springframework.web.bind.annotation.GetMapping", 1)
	assertEdge(t, edges, "DEPENDS_ON_EXTERNAL", "ledger", .95)
	assertEdge(t, edges, "DECLARES_ENDPOINT", "/accounts", .9)
	assertEdge(t, edges, "INVOKES_API", "https://accounts", .95)
	assertEdge(t, edges, "CONSUMES_TOPIC", "account-events", .95)
	assertEdge(t, edges, "PRODUCES_TOPIC", "account-events", .95)
	assertEdge(t, edges, "EMITS_LOG", "received", .95)
}

func TestJavaFactsAndSpringEdges(t *testing.T) {
	content := `package example;
import java.util.List;
@RestController
@RequestMapping("/accounts")
public class AccountController {
  @GetMapping("/{id}")
  public String getAccount() { return "ok"; }
  @KafkaListener(topics = "account-events")
  public void consume() {}
}`
	symbols, edges, err := Parse(model.File{RepoID: "account", Path: "AccountController.java", Language: "java", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	assertSymbol(t, symbols, "AccountController", "class")
	assertSymbol(t, symbols, "getAccount", "method")
	assertSymbol(t, symbols, "consume", "method")
	assertEdge(t, edges, "IMPORTS", "java.util.List", 1)
	assertEdge(t, edges, "DECLARES_ENDPOINT", "/accounts", .9)
	assertEdge(t, edges, "DECLARES_ENDPOINT", "/{id}", .9)
	assertEdge(t, edges, "CONSUMES_TOPIC", "account-events", .95)
}

func TestJavaEventPublicationAndConsumptionRequireExplicitIdioms(t *testing.T) {
	producer := `class CustomerChangedService {
  void customerChanged(Object customerChanged) {
    jmsTemplate.convertAndSend("example.accounts.events", customerChanged);
    applicationEventPublisher.publishEvent(new CustomerChangedEvent(customerChanged));
  }
}`
	_, edges, err := Parse(model.File{RepoID: "customer", Path: "CustomerChangedService.java", Language: "java", Content: producer})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "EMITS_EVENT", "example.accounts.events", .9)
	assertEdge(t, edges, "EMITS_EVENT", "CustomerChangedEvent", .82)
	consumer := `class AccountConsumer {
  @JmsListener(destination = "example.accounts.events")
  void onCustomerChanged(String body) {}
}`
	_, edges, err = Parse(model.File{RepoID: "account", Path: "AccountConsumer.java", Language: "java", Content: consumer})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "CONSUMES_EVENT", "example.accounts.events", .9)
	// Same words without a supported listener/publication idiom are not evidence.
	_, edges, err = Parse(model.File{RepoID: "account", Path: "Notes.java", Language: "java", Content: `class Notes { void customerChanged() { String queue = "example.accounts.events"; } }`})
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range edges {
		if edge.Kind == "EMITS_EVENT" || edge.Kind == "CONSUMES_EVENT" {
			t.Fatalf("unsupported name/string inference: %#v", edge)
		}
	}
}

func TestExplicitTypedRelationshipExtraction(t *testing.T) {
	_, edges, err := Parse(model.File{RepoID: "ui", Path: "src/api.ts", Language: "typescript", Content: `function load() { return fetch("/api/accounts"); }`})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "INVOKES_API", "/api/accounts", .95)
	assertEdge(t, edges, "INVOKES_API", "GET /api/accounts", .95)
	_, edges, err = Parse(model.File{RepoID: "service", Path: "src/A.java", Language: "java", Content: `class A { void load() { restTemplate.getForObject("https://accounts", String.class); } }`})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "INVOKES_API", "https://accounts", .9)
	assertEdge(t, edges, "INVOKES_API", "GET https://accounts", .9)
	_, edges, err = Parse(model.File{RepoID: "service", Path: "application.yml", Language: "yaml", Classification: "configuration", Content: "account.endpoint: https://accounts\n"})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "DEFINES_CONFIGURATION", "account.endpoint", .95)
}

func TestPackageCoordinatesAreDirectManifestFacts(t *testing.T) {
	_, edges, err := Parse(model.File{RepoID: "ui", Path: "package.json", Language: "text", Content: `{"name":"@example/ui","dependencies":{"@example/contracts":"1"}}`})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "DECLARES_MODULE", "@example/ui", .95)
	assertEdge(t, edges, "DEPENDENCY_DECLARATION", "@example/contracts", .95)
}

func TestRelationshipFamiliesUseExplicitSyntaxNotNamesOrComments(t *testing.T) {
	java := `
@FeignClient(name = "ledger") interface LedgerClient {}
class Integration {
  @Value("${ledger.base-url}") String ledgerURL;
  @EventListener(CustomerChanged.class) void observed(CustomerChanged event) {}
  void run() {
    jmsTemplate.convertAndSend("customer.queue", new CustomerChanged());
    kafkaTemplate.send("customer.topic", "body");
    applicationEventPublisher.publishEvent(new CustomerChanged());
    jdbcTemplate.query("select id from customer_account", rowMapper);
    jdbcTemplate.update("update customer_account set enabled = true");
    // kafkaTemplate.send("comment.topic", "not a call");
  }
  void notes() { String Producer = "customer.queue"; }
}`
	_, edges, err := Parse(model.File{RepoID: "service", Path: "Integration.java", Language: "java", Content: java})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "DEPENDS_ON_EXTERNAL", "ledger", .95)
	assertEdge(t, edges, "REFERENCES_CONFIGURATION", "ledger.base-url", .95)
	assertEdge(t, edges, "LISTENS_EVENT", "CustomerChanged", .95)
	assertEdge(t, edges, "SENDS_QUEUE", "customer.queue", .95)
	assertEdge(t, edges, "PRODUCES_TOPIC", "customer.topic", .95)
	assertEdge(t, edges, "PUBLISHES_EVENT", "CustomerChanged", .95)
	assertEdge(t, edges, "READS_TABLE", "customer_account", .9)
	assertEdge(t, edges, "WRITES_TABLE", "customer_account", .9)
	for _, e := range edges {
		if e.Target == "comment.topic" || (e.Target == "customer.queue" && e.Source == "notes") {
			t.Fatalf("comment/name became a relationship: %#v", e)
		}
	}

	ui := `
import axios from "axios";
export function AccountsPage() { return <Route path="/accounts" element={<Accounts/>} />; }
export async function loadAccounts() { return axios.get("/api/accounts"); }
// fetch("/api/comment")
`
	_, edges, err = Parse(model.File{RepoID: "ui", Path: "Accounts.tsx", Language: "tsx", Content: ui})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "DECLARES_UI_ROUTE", "/accounts", .95)
	assertEdge(t, edges, "INVOKES_API", "/api/accounts", .95)
	for _, e := range edges {
		if e.Target == "/api/comment" {
			t.Fatalf("comment became API call: %#v", e)
		}
	}

	_, edges, err = Parse(model.File{RepoID: "service", Path: "application.properties", Language: "properties", Classification: "configuration", Content: "ledger.base-url=https://ledger\nclient.url=${ledger.base-url}/v1\n"})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "DEFINES_CONFIGURATION", "ledger.base-url", .95)
	assertEdge(t, edges, "REFERENCES_CONFIGURATION", "ledger.base-url", .95)
}

func TestJavaDomainFactsIncludeEffectiveRoutesGuardsLogsAndCoverageGaps(t *testing.T) {
	content := `@RequestMapping("/accounts") class Controller {
  @GetMapping("/{id}") void get() { if (enabled) { kafkaTemplate.send("accounts", "x"); logger.error("missing"); } }
  @KafkaListener(topics = TOPIC) void dynamic() {}
  @ConfigurationProperties(prefix = "client") Object client;
  @Table(name = "account") class Account {}
  void fail() { throw new IllegalStateException(); }
}`
	_, edges, err := Parse(model.File{RepoID: "repo", Path: "Controller.java", Language: "java", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "DECLARES_EFFECTIVE_ENDPOINT", "GET /accounts/{id}", .9)
	assertEdge(t, edges, "PRODUCES_TOPIC", "accounts", .95)
	assertEdge(t, edges, "EMITS_ERROR", "missing", .95)
	assertEdge(t, edges, "EMITS_ERROR", "IllegalStateException", .95)
	assertEdge(t, edges, "BINDS_CONFIGURATION", "client", .95)
	assertEdge(t, edges, "MAPS_TABLE", "account", .95)
	assertEdge(t, edges, "COVERAGE_GAP", "dynamic_kafkalistener", 0)
	assertEdge(t, edges, "HAS_LOCAL_GUARD", "enabled", 1)
}

func TestTypeScriptDomainFactsIncludeRoutesComponentsEventsConfigAndGuards(t *testing.T) {
	content := `function Accounts() { return null; }
function App() { return <Route path="/accounts" element={<Accounts/>} />; }
function load(url: string) { if (enabled) { fetch("/api/accounts"); console.error("failed"); } fetch(url); window.dispatchEvent(new CustomEvent("accounts.loaded")); window.addEventListener("accounts.loaded", handler); return process.env.API_URL + import.meta.env.VITE_REGION; }
function fail() { throw new Error(); }`
	_, edges, err := Parse(model.File{RepoID: "ui", Path: "src/App.tsx", Language: "tsx", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	assertEdge(t, edges, "DECLARES_UI_ROUTE", "/accounts", .95)
	assertEdge(t, edges, "ROUTE_RENDERS_COMPONENT", "Accounts", .95)
	assertEdge(t, edges, "INVOKES_API", "/api/accounts", .95)
	assertEdge(t, edges, "EMITS_ERROR", "failed", .95)
	assertEdge(t, edges, "PUBLISHES_EVENT", "accounts.loaded", .95)
	assertEdge(t, edges, "SUBSCRIBES_EVENT", "accounts.loaded", .95)
	assertEdge(t, edges, "REFERENCES_CONFIGURATION", "API_URL", .95)
	assertEdge(t, edges, "REFERENCES_CONFIGURATION", "VITE_REGION", .95)
	assertEdge(t, edges, "EMITS_ERROR", "Error", .95)
	assertEdge(t, edges, "HAS_LOCAL_GUARD", "enabled", 1)
	assertEdge(t, edges, "COVERAGE_GAP", "nonliteral_http_url", 0)
}

func assertSymbol(t *testing.T, symbols []model.Symbol, name, kind string) {
	t.Helper()
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Kind == kind && symbol.Span.StartLine > 0 && symbol.Confidence == 1 {
			return
		}
	}
	t.Fatalf("missing symbol %s/%s in %#v", name, kind, symbols)
}

func assertEdge(t *testing.T, edges []model.Edge, kind, target string, confidence float64) {
	t.Helper()
	for _, edge := range edges {
		if edge.Kind == kind && edge.Target == target && edge.Confidence == confidence && edge.Resolver != "" && edge.Span.StartLine > 0 {
			return
		}
	}
	t.Fatalf("missing edge %s/%s in %#v", kind, target, edges)
}
