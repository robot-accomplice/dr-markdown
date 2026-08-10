package e2e

import "testing"

// A tall empty state must stay reachable.
//
// #document-region scrolls and centres its child on the cross axis. A flex item
// centred that way which outgrows its container overflows in BOTH directions,
// and the part above the top edge cannot be scrolled to — scrollTop is already
// 0. The empty state grows with the recent-files list, so this appears once a
// user has enough recents and not before, which is why nobody hit it early.
//
// Observed on the shipped layout at a 1440x900 window with seven recent files:
// an 798px block inside a 750px region, its top 53px above the region's own.
// The heading and logo were simply gone with no way to reach them.
//
// The height is forced here rather than by seeding recents: the recents list
// comes from Go preferences and varies per machine, and a layout test that
// depends on how many files someone happened to open is not a test.
func TestTallEmptyStateStaysReachable(t *testing.T) {
	ctx, cancel := newTestBrowser(t)
	defer cancel()
	url := serveFrontend(t)
	bootApp(t, ctx, url)

	var got struct {
		RegionTop    float64 `json:"regionTop"`
		EmptyTop     float64 `json:"emptyTop"`
		EmptyHeight  float64 `json:"emptyHeight"`
		RegionHeight float64 `json:"regionHeight"`
		ScrollTop    float64 `json:"scrollTop"`
	}
	evalJS(t, ctx, `(async () => {
	  const region = document.querySelector('#document-region')
	  const empty = document.querySelector('#empty-state')

	  // Force the overflow the recents list causes in the wild.
	  const filler = document.createElement('div')
	  filler.style.height = '600px'
	  empty.appendChild(filler)
	  await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))

	  const r = region.getBoundingClientRect()
	  const e = empty.getBoundingClientRect()
	  const out = {
	    regionTop: r.top, emptyTop: e.top,
	    emptyHeight: e.height, regionHeight: r.height,
	    scrollTop: region.scrollTop,
	  }
	  filler.remove()
	  return out
	})()`, &got)

	if got.EmptyHeight <= got.RegionHeight {
		t.Fatalf("the fixture did not overflow the region (%v <= %v), so this test proves nothing",
			got.EmptyHeight, got.RegionHeight)
	}

	// The top of the content must not sit above the top of the thing that
	// scrolls it. One pixel of tolerance for sub-pixel layout.
	if got.EmptyTop < got.RegionTop-1 {
		t.Errorf("the empty state starts %.0fpx above its scroll container and cannot be reached: "+
			"empty top %.0f, region top %.0f, scrollTop %.0f. "+
			"Cross-axis centring needs `safe` when the container scrolls.",
			got.RegionTop-got.EmptyTop, got.EmptyTop, got.RegionTop, got.ScrollTop)
	}
}
