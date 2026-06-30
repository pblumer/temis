// Package assist is a provider-agnostic, tool-calling LLM client that powers the
// modeler's built-in modeling assistant (ADR-0024). It inverts the Agent-First
// relationship of ADR-0013: instead of being the tool an external agent calls,
// temis here is the host that calls an LLM and lets it drive a small set of
// temis tools to help a human build decisions.
//
// The package is deliberately thin and free of external dependencies (pure
// standard library, consistent with ADR-0014). It has three parts:
//
//   - Provider is one LLM backend (Anthropic, OpenAI). It performs a single,
//     non-streaming model turn given a system prompt, the conversation so far and
//     a tool catalog, and reports back the model's text and/or tool-call requests.
//     Concrete providers live in the assist/anthropic and assist/openai
//     subpackages, each a stdlib net/http client.
//
//   - Executor is the tool surface the model may drive. Like vcs.Reader
//     (ADR-0022), it is an interface implemented by the caller (the service), so
//     this package stays free of service- and internal-package concerns. Its
//     tools mirror temis's existing operations (inspect models, evaluate, edit
//     decision tables) so the assistant can verify its own suggestions against
//     the real engine rather than guess.
//
//   - Agent runs the tool-calling loop: it asks the provider, runs any requested
//     tools through the Executor, feeds the results back, and repeats until the
//     model produces a final text answer or a bounded step budget is exhausted
//     (hostile-input discipline, golden rule 7).
package assist
