# prompt — your prompt, your rules

`Env PS1` was always legal `.tape` grammar; foley makes it actually
WIN over the pinned shell prompt, and a bare `Wait` learns the new
prompt automatically (its pattern derives from the PS1's static tail —
when the prompt is fully dynamic, foley says so loudly and tells you
to pair it with `Set WaitPattern`).

```tape
Env PS1 "\[\e[35m\]❯\[\e[0m\] "   # colored ❯ — colors are fine
Type "echo mi prompt, mis reglas"
Enter
Wait                               # syncs against ❯ automatically
```

## Which shells support it

A custom prompt works where the prompt IS a variable:

| Shell        | Variable     | Custom prompt |
|--------------|--------------|---------------|
| `bash`       | `Env PS1`    | ✓             |
| `osh`        | `Env PS1`    | ✓             |
| `zsh`        | `Env PROMPT` | ✓             |
| `fish`       | function     | ✗ (pinned)    |
| `nu`         | function     | ✗ (pinned)    |
| `xonsh`      | function     | ✗ (pinned)    |
| `powershell` | function     | ✗ (pinned)    |
| `pwsh`       | function     | ✗ (pinned)    |

The function-prompt shells define their prompt in code, not an
environment variable — setting `Env PS1` there does nothing, and foley
tells you exactly that instead of pretending. `foley validate` reports
the coordination for any tape before recording.

Degradability: the same tape still records in VHS — there the `Env`
override has no effect and it falls back to VHS's own prompt.

If your prompt's tail is dynamic (powerline segments, `$(git ...)` at
the very end), derive fails LOUDLY and bare `Wait` keeps the default
`>$` — pair the prompt with an explicit `Set WaitPattern` in that case.
