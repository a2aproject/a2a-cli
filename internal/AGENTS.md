### General

* Keep the CLI package clean, create a file-per-command. Try extracting logic into a different package.
* Ask clarifying questions from the user if details important for the task are missing.
* When working on a bug fix, follow the RED-GREEN-BLUE TDD approach.

### Code Style

* Do not leave comments in the code unless they explain a non-trivial implementation detail or highlight a suboptimally handled edge-case.
* Prefer early `return`-s and `continue` over deeply nested blocks.
* Never ignore error results `_ = smth.Close()`, errors need to bubble up OR be logged.

### Test Writing

* Do not leave comments in tests unless they explain a non-trivial implementation detail. Test name and test case setup should be self-explanatory.
* ALWAYS use existing test files as a reference when generating new tests. Prioritize files in the same package if they exist.
* Write "table-driven tests" when logic can be shared across multiple test cases.
* Prioritize testing observable behavior of exported methods, not the internal state.
* Use `google/go-cmp/cmp` and `cmp.Diff(want, got)` for comparing structs, slices, maps, or complex types. Do NOT use `reflect.DeepEqual` or manual field checks.
* Use existing mocking utilities or create a new utility to keep test code clean.
* Use `t.Parallel()` at the start of test cases.
* Use `receiver.Operation() error = %v, want %v` as a template for printing test error check failures.
* Use `receiver.Operation() = %v, want %v` as a template for printing test error check failures.
* Use `receiver.Operation() wrong result (-want +got) diff = %s` as a template for printing test errors received when using `cmp.Diff`.
* Prefer using `t.Fatalf` over `t.Errorf` unless printing all the failed checks is justified or the method is called not from the main goroutine. 