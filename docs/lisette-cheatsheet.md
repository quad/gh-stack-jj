# Lisette cheatsheet

This project uses Lisette 0.11.2. The language compiles to Go, keeps Go
interoperability, and uses `Option` and `Result` instead of `nil` and ignored
errors.

References: [language overview](https://lisette.run/), [reference index](https://github.com/ivov/lisette/blob/main/docs/reference/README.md), [safety guide](https://github.com/ivov/lisette/blob/main/docs/intro/safety.md).

## Project commands

Each directory under `src` is a package. `src/main.lis` is the executable
entrypoint. A project import names another package; a `go:` import names a Go
package.

```sh
lis format             # format source
lis check              # type-check and lint
lis test               # run *.test.lis files
lis build              # build the project
lis emit               # inspect generated Go when needed
```

## Expressions and bindings

Functions require argument types. The return type is optional. The last
expression is the return value; `return` is for early exits.

```lis
fn title(layer: Layer) -> string { layer.title }

let base = "main"
let mut values: Slice<string> = []
values = values.append("one")

let result = if ready { "yes" } else { "no" }
```

Bindings are immutable unless declared `mut`. Lisette has no `nil` or
exception-based error handling. Use `Option<T>` for absence and `Result<T, E>`
for failure.

## Control flow

`if` is an expression. `if let` handles one pattern. `let else` handles the
failure path and leaves the bound values available afterward. `match` is an
expression and must be exhaustive.

```lis
if let Some(value) = maybe_value { use(value) }

let Some(value) = maybe_value else { return Err(err) }

let label = match action {
  Action.Noop => "no-op",
  Action.Append => "append",
  Action.Restructure => "restructure",
}
```

`for` and `while` return `()`. `loop` can return a value with `break value`.

## Structs, enums, and methods

```lis
struct Layer { id: string, title: string }

enum Action { Noop, Append, Restructure }

let layer = Layer { id: id, title: title }

impl Layer {
  fn is_empty(self) -> bool { self.title == "" }
}
```

Struct construction supports field shorthand and spread:
`Layer { id, ..old_layer }`. `impl` also supports associated functions. A
value receiver is immutable; use `Ref<T>` when a method must mutate a value.

Interfaces are structural. Use them for small dependencies such as this
project's `Runner`; concrete structs are usually simpler for data.

## Option and Result

Use `?` to return an error or propagate an absent value. Add context with
`wrap_err` when the boundary needs a useful operation name.

```lis
fn load() -> Result<Value, error> {
  let text = read()?
  parse(text).wrap_err("parse response")
}

let value = option.map(|item| item.name()).unwrap_or("default")
```

Other direct forms are `match`, `if let`, and `let else`. Do not add an
`unwrap`-style helper to avoid handling a case: use `unwrap_or` when a default
is correct, or return an error.

## Slices and pipelines

`Slice<T>` is the usual collection. Its methods return new values where
appropriate; assign the result of `append`.

```lis
let names = layers.filter(|layer| layer.title != "").map(|layer| layer.title)
let numbers = names.map(|name| parse_number(name))
let text = names.join(", ")

let first = values.get(0)       // Option<string>
let found = values.find(|v| v == wanted)
let total = values.fold(0, |sum, value| sum + value)
```

Useful operations include `map`, `filter`, `find`, `fold`, `contains`,
`equals`, `join`, `append`, `get`, `length`, and `is_empty`.

The pipeline operator passes its left side as the first argument of a function
call. The right side must be a call, not a bare lambda:

```lis
let result = input |> parse() |> validate()
```

Prefer a short collection expression over a manual loop. Keep a loop when it
has clearer early exits or error handling.

## Go interop and JSON

Import Go packages with `go:` and call exported Go names directly:

```lis
import "go:encoding/json"
import "go:strings"

let decoder = json.NewDecoder(strings.NewReader(input))
json.Unmarshal(input as Slice<byte>, &value)?
```

Common Go boundary conversions are `(T, error)` to `Result<T, error>` and
`(T, bool)` to `Option<T>`. Check the actual generated/interop type when a Go
API has several return values.

`#[json]` generates JSON tags. Use field options such as `omitempty`, `skip`,
`snake_case`, `camel_case`, or an explicit name when the wire format needs it.
Keep Lisette fields snake_case; the generated Go values remain exported and
match the `gh` and `jj` payloads.

## Tests

Put tests in `*.test.lis` files and mark functions with `#[test]`:

```lis
#[test]
fn empty_input_is_ok() {
  assert(parse("[]").is_ok())
}
```

Use a fake `Runner` with exact queued commands for workflow tests. Make the
fake fail when the queue is exhausted; that exposes unexpected subprocesses
instead of silently returning an empty response.

## Simplification rules for this codebase

- Prefer `if let` or `let else` when there are only two cases; reserve `match`
  for real branching or enum exhaustiveness.
- Prefer `map`, `filter`, `find`, `fold`, `join`, and `|>` when they make the
  data flow clearer. Do not abstract a one-use transformation.
- Keep `Result` propagation at the boundary that can add useful context.
- Keep Go interop structs explicit; use small internal structs when wire names
  do not belong in the rest of the program.
- Use `Slice<T>` methods rather than hand-written indexing, and remember that
  `.get` returns `Option<T>`.
- Verify unfamiliar syntax with `lis check`; the pinned compiler is the
  authority over examples from newer Lisette documentation.
