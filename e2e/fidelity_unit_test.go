package e2e

import (
	"encoding/json"
	"testing"
)

// The fidelity modules are pure — no DOM, no Crepe, no bridge — so they can be
// imported into one already-served page and exercised directly, instead of
// booting a browser per behaviour the way the rest of the frontend suite must.
// That is the point of the extraction, so it gets tests that depend on it.

// Every registered module must satisfy its port. A module that half-implements
// one would otherwise fail deep inside a serialize call, with the symptom
// (mangled markdown) far from the cause (a missing restore function).
func TestFidelityRegistryContract(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var out json.RawMessage
	evalJS(t, ctx, `(async () => {
		const f = await import('/src/fidelity/index.js')
		const problems = []
		if (!Array.isArray(f.PRESERVATIONS) || f.PRESERVATIONS.length === 0) problems.push('PRESERVATIONS missing or empty')
		if (!Array.isArray(f.RESTORE_SEQUENCE)) problems.push('RESTORE_SEQUENCE missing')
		if (!Array.isArray(f.SERIALIZER_POLICIES)) problems.push('SERIALIZER_POLICIES missing')
		for (const p of f.PRESERVATIONS || []) {
			if (typeof p.name !== 'string' || !p.name) problems.push('a preservation has no name')
			if (typeof p.capture !== 'function') problems.push(p.name + ': capture is not a function')
			if (typeof p.restore !== 'function') problems.push(p.name + ': restore is not a function')
		}
		for (const p of f.SERIALIZER_POLICIES || []) {
			if (typeof p.name !== 'string' || !p.name) problems.push('a policy has no name')
			if (typeof p.detect !== 'function') problems.push(p.name + ': detect is not a function')
		}
		// Both sequences must contain exactly the same modules. A preservation
		// captured but never restored silently drops the user's bytes.
		const cap = (f.PRESERVATIONS || []).map((p) => p.name).sort().join(',')
		const res = (f.RESTORE_SEQUENCE || []).map((p) => p.name).sort().join(',')
		if (cap !== res) problems.push('capture set != restore set: [' + cap + '] vs [' + res + ']')
		return problems
	})()`, &out)

	var problems []string
	if err := json.Unmarshal(out, &problems); err != nil {
		t.Fatalf("registry probe returned %s: %v", out, err)
	}
	for _, p := range problems {
		t.Errorf("registry contract: %s", p)
	}
}

// Each preservation is pure, so it can be driven directly with a table instead
// of through a browser round trip. One boot for the whole table.
func TestTrailingPreservation(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got []string
	evalJS(t, ctx, `(async () => {
		const { trailing } = await import('/src/fidelity/trailing.js')
		const run = (original, serialized) => {
			const { state } = trailing.capture(original)
			return trailing.restore(serialized, state)
		}
		return [
			run('# T\n\n- a\n', '# T\n\n- a\n\n'),
			run('# T\n', '# T\n'),
			run('no newline at eof', 'no newline at eof\n'),
			run('# T\n\n\n', '# T\n'),
		]
	})()`, &got)

	want := []string{"# T\n\n- a\n", "# T\n", "no newline at eof", "# T\n\n\n"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("case %d: got %q want %q", i, got[i], want[i])
		}
	}
}
