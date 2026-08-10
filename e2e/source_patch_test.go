package e2e

import "testing"

// probeCrepeJS creates a throwaway Crepe on a detached div and returns it.
//
// This is the method the 2026-08-09 source-positions probe used, and it is
// chosen so the probe touches nothing the app runs: no import of editor.js, no
// fidelity registry, no __app surface. The instance exists only to hand over
// `remark`, which the bundle does not export but the live ctx does.
const probeCrepeJS = `
  async function newProbeCrepe(markdown) {
    const { Crepe } = await import('/vendor/crepe.bundle.mjs')
    const host = document.createElement('div')
    document.body.appendChild(host)
    const crepe = new Crepe({ root: host, defaultValue: markdown })
    await crepe.create()
    return { crepe, host }
  }
`

// The approach diffs SYNTAX TREES rather than texts, and that only closes the
// re-serialization class if the respelling lives in the serializer rather than
// in the tree. Two claims carry the whole design:
//
//  1. a table's mdast is invariant under delimiter-row padding
//  2. character references are decoded at parse time
//
// Both were predicted, neither measured. This project has three times recorded
// a conclusion with true premises that was false anyway, so they are measured
// here BEFORE any diff code exists. If this test fails, the approach is dead.
func TestRespellingLivesInTheSerializerNotTheTree(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got struct {
		TableEqual  bool   `json:"tableEqual"`
		EntityValue string `json:"entityValue"`
		TablePadded string `json:"tablePadded"`
		TableTerse  string `json:"tableTerse"`
	}
	evalJS(t, ctx, probeCrepeJS+`(async () => {
	  const { crepe, host } = await newProbeCrepe('x')
	  const remark = crepe.editor.ctx.get('remark')

	  const strip = (node) => JSON.stringify(node, (k, v) => (k === 'position' ? undefined : v))
	  const padded = remark.parse('| a | b |\n| --- | --- |\n| 1 | 2 |\n')
	  const terse  = remark.parse('| a | b |\n| - | - |\n| 1 | 2 |\n')
	  const entity = remark.parse('&amp;\n')

	  const result = {
	    tableEqual: strip(padded) === strip(terse),
	    entityValue: entity.children[0].children[0].value,
	    tablePadded: strip(padded),
	    tableTerse: strip(terse),
	  }
	  await crepe.destroy().catch(() => {})
	  host.remove()
	  return result
	})()`, &got)

	if !got.TableEqual {
		t.Errorf("PREDICTION 1 REFUTED: delimiter-row padding changes the mdast, so a structural "+
			"diff cannot preserve it. The approach is dead; record this and stop.\npadded: %s\nterse:  %s",
			got.TablePadded, got.TableTerse)
	}
	if got.EntityValue != "&" {
		t.Errorf("PREDICTION 2 REFUTED: `&amp;` parses to %q, not \"&\", so entity decoding is not "+
			"a parse-time normalization and the tree is not respelling-invariant", got.EntityValue)
	}
}
