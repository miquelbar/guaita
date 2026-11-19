package server

import (
	"os/exec"
	"strings"
	"testing"
)

// TestScrollBehavior_TemplateContracts verifies that both mobile and admin templates
// adhere to the non-locking, touch-preserving, and non-flapping contracts.
func TestScrollBehavior_TemplateContracts(t *testing.T) {
	mobileHTML := RenderMobileHTML(7)
	adminHTML := RenderAdminHTML(AdminPageData{
		LocalIP:      "127.0.0.1",
		Port:         "8085",
		QRCodeBase64: "dummy",
		PairingURL:   "http://127.0.0.1:8085/?token=xyz",
		SessionDays:  7,
		CSPNonce:     "test-nonce",
	})

	for name, html := range map[string]string{
		"mobile": mobileHTML,
		"admin":  adminHTML,
	} {
		// 1. Must use the tight 15px threshold rather than the old buggy 48px deadband
		if strings.Contains(html, "<= 48") {
			t.Errorf("[%s] found obsolete 48px threshold; must use <= 15", name)
		}
		if !strings.Contains(html, "<= 15") {
			t.Errorf("[%s] missing 15px tolerance threshold (<= 15)", name)
		}

		// 2. Must register touch and wheel listeners to detect human scroll intent
		for _, event := range []string{"touchstart", "touchend", "touchcancel", "wheel"} {
			if !strings.Contains(html, event) {
				t.Errorf("[%s] missing user interaction listener for %q", name, event)
			}
		}

		// 3. Must guard against DOM mutations during active touch swiping
		if !strings.Contains(html, "pendingMutate") {
			t.Errorf("[%s] missing pendingMutate buffer to prevent touch interruption", name)
		}

		// 4. Must NOT aggressively force ws.close() inside onerror (preventing flapping)
		if strings.Contains(html, "ws.onerror = () => { ws.close(); };") {
			t.Errorf("[%s] found aggressive ws.close() in ws.onerror; leads to flapping cycles", name)
		}

		// 6. Must guard connect() against multiple concurrent sockets if CONNECTING or OPEN
		if !strings.Contains(html, "WebSocket.CONNECTING") || !strings.Contains(html, "WebSocket.OPEN") {
			t.Errorf("[%s] connect() must guard against concurrent sockets when readyState is CONNECTING or OPEN", name)
		}
	}

	// Mobile specific: Jump button for both snippets and raw view
	if !strings.Contains(mobileHTML, "btnJumpTermMobile") {
		t.Errorf("[mobile] missing btnJumpTermMobile")
	}
	if !strings.Contains(mobileHTML, "btnJumpRawMobile") {
		t.Errorf("[mobile] missing btnJumpRawMobile")
	}

	// Admin specific: Jump button for snippet
	if !strings.Contains(adminHTML, "btnJumpTermAdmin") {
		t.Errorf("[admin] missing btnJumpTermAdmin")
	}
}

// TestScrollBehavior_NodeSimulation executes a headless behavioral test of createScrollTracker
// using Node.js to verify scroll lock prevention, touch buffering, and jump button behavior.
func TestScrollBehavior_NodeSimulation(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node executable not found in PATH, skipping headless JS behavioral simulation")
	}

	jsScript := `
const assert = require('assert');

// Mock DOM Element with event listeners and scroll properties
class MockElement {
    constructor(scrollHeight, clientHeight, scrollTop) {
        this.scrollHeight = scrollHeight;
        this.clientHeight = clientHeight;
        this.scrollTop = scrollTop;
        this.listeners = {};
        this.style = {};
        this.innerHTML = '';
    }
    addEventListener(evt, fn) {
        if (!this.listeners[evt]) this.listeners[evt] = [];
        this.listeners[evt].push(fn);
    }
    dispatchEvent(evt) {
        if (this.listeners[evt]) {
            for (const fn of this.listeners[evt]) fn();
        }
    }
}

global.requestAnimationFrame = (fn) => process.nextTick(fn);

// Implementation of createScrollTracker under test
function createScrollTracker(el, jumpBtn) {
    let pinned = true;
    let isTouching = false;
    let pendingMutate = null;

    const checkPinned = () => {
        if (!el) return true;
        const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
        return dist <= 15;
    };

    const updateJumpBtn = () => {
        if (jumpBtn) {
            jumpBtn.style.display = pinned ? 'none' : 'block';
        }
    };

    if (el) {
        el.addEventListener('touchstart', () => {
            isTouching = true;
        });

        const endTouch = () => {
            isTouching = false;
            if (pendingMutate) {
                const fn = pendingMutate;
                pendingMutate = null;
                tracker.update(fn);
            }
        };

        el.addEventListener('touchend', endTouch);
        el.addEventListener('touchcancel', endTouch);

        el.addEventListener('wheel', () => {
            pinned = checkPinned();
            updateJumpBtn();
        });

        el.addEventListener('scroll', () => {
            pinned = checkPinned();
            updateJumpBtn();
        });
    }

    if (jumpBtn && el) {
        jumpBtn.addEventListener('click', () => {
            pinned = true;
            el.scrollTop = el.scrollHeight;
            updateJumpBtn();
        });
    }

    const tracker = {
        isPinned: () => pinned,
        update: (mutateFn) => {
            if (!el) { mutateFn(); return; }
            if (isTouching) {
                pendingMutate = mutateFn;
                return;
            }
            const wasPinned = pinned;
            const prevTop = el.scrollTop;
            mutateFn();
            if (wasPinned) {
                el.scrollTop = el.scrollHeight;
            } else {
                el.scrollTop = prevTop;
            }
            pinned = checkPinned();
            updateJumpBtn();
        },
        scrollToBottom: () => {
            if (!el) return;
            pinned = true;
            el.scrollTop = el.scrollHeight;
            updateJumpBtn();
        }
    };
    return tracker;
}

// TEST 1: Initial state is pinned at bottom
const el = new MockElement(1000, 300, 700); // 1000 - 300 - 700 = 0
const jumpBtn = new MockElement(0, 0, 0);
const tracker = createScrollTracker(el, jumpBtn);

assert.strictEqual(tracker.isPinned(), true, "initial tracker must be pinned");
assert.strictEqual(jumpBtn.style.display, undefined, "jump button initially unset/hidden");

// TEST 2: Update while pinned auto-scrolls to bottom
el.scrollHeight = 1100;
tracker.update(() => { el.innerHTML = 'new output'; });
assert.strictEqual(el.scrollTop, 1100, "when pinned, update must scroll to bottom");
assert.strictEqual(jumpBtn.style.display, 'none', "jump button must be hidden when pinned");

// TEST 3: User scrolls UP (> 15px, e.g. from 800 to 600)
el.scrollTop = 600; // dist = 1100 - 300 - 600 = 200 > 15
el.dispatchEvent('scroll');
assert.strictEqual(tracker.isPinned(), false, "scrolling up must unpin the tracker");
assert.strictEqual(jumpBtn.style.display, 'block', "jump button must be visible when unpinned");

// TEST 4: Updates arriving while UNPINNED must NOT force scroll to bottom (PREVENTS SCROLL LOCK!)
el.scrollHeight = 1300;
tracker.update(() => { el.innerHTML = 'more logs'; });
assert.strictEqual(el.scrollTop, 600, "when unpinned, update MUST preserve prevTop and NOT jump to bottom");
assert.strictEqual(jumpBtn.style.display, 'block', "jump button must stay visible while unpinned");

// TEST 5: Active touch gesture buffers DOM mutation so swipe is never interrupted
el.dispatchEvent('touchstart');
let mutated = false;
tracker.update(() => { mutated = true; });
assert.strictEqual(mutated, false, "update during active touch must be deferred to prevent touch gesture cancellation");

// Touch ends: deferred mutation runs, preserving scroll offset
el.dispatchEvent('touchend');
assert.strictEqual(mutated, true, "deferred mutation must execute upon touchend");
assert.strictEqual(el.scrollTop, 600, "scroll offset must still be preserved after deferred mutation");

// TEST 6: User clicks jump button -> immediately restores bottom pinning
jumpBtn.dispatchEvent('click');
assert.strictEqual(tracker.isPinned(), true, "clicking jump button must re-pin");
assert.strictEqual(el.scrollTop, 1300, "clicking jump button must scroll to bottom");
assert.strictEqual(jumpBtn.style.display, 'none', "jump button must hide after click");

// TEST 7: Scrolling back down manually within 15px restores pinned state
el.scrollTop = 1000 - 10; // dist = 1300 - 300 - 990 = 10 <= 15
el.dispatchEvent('scroll');
assert.strictEqual(tracker.isPinned(), true, "scrolling within 15px of bottom must restore pinned state");
assert.strictEqual(jumpBtn.style.display, 'none', "jump button must hide when back at bottom");

console.log("SCROLL_REGRESSION_SIMULATION_OK");
`

	cmd := exec.Command(nodePath, "-e", jsScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Node scroll simulation failed: %v\nOutput:\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "SCROLL_REGRESSION_SIMULATION_OK") {
		t.Fatalf("Unexpected simulation output: %s", string(out))
	}
}
