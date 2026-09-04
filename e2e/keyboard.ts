// The keyboard-walking primitives e2e/a11y.spec.ts uses. Split out of
// smoke.spec.ts's own copy (which stays exactly as it was, scoped to `main`
// alone for its two record-page cases) rather than imported from it, because
// a11y.spec.ts needs to start from the whole document — the shell's own nav
// and skip link are themselves controls SC-014 covers, on every page rather
// than on the two smoke.spec.ts already drives.
import type { Page } from '@playwright/test';

export type Focused = {
  tag: string;
  id: string;
  label: string;
  indicator: boolean;
};

// describe is how a control seen in the DOM and the same control seen under
// focus are recognised as one thing, without marking up the page to do it.
export function describe(control: Focused): string {
  return `${control.tag}#${control.id}:${control.label}`;
}

const focusable =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export async function focusableControls(page: Page, within: string): Promise<string[]> {
  return page.evaluate(
    ([selector, scope]) => {
      const root = document.querySelector(scope);
      if (!root) return [];

      return Array.from(root.querySelectorAll<HTMLElement>(selector))
        .filter((element) => element.offsetParent !== null || element.tagName === 'A')
        .map((element) => {
          const label = (element.getAttribute('aria-label') ?? element.textContent ?? '').trim().slice(0, 40);
          return `${element.tagName.toLowerCase()}#${element.id}:${label}`;
        });
    },
    [focusable, within] as const,
  );
}

export async function tabThrough(page: Page, steps: number): Promise<Focused[]> {
  const walk: Focused[] = [];

  for (let step = 0; step < steps; step += 1) {
    await page.keyboard.press('Tab');

    const control = await page.evaluate(() => {
      const element = document.activeElement as HTMLElement | null;
      if (!element || element === document.body) return null;

      const style = getComputedStyle(element);

      return {
        tag: element.tagName.toLowerCase(),
        id: element.id,
        label: (element.getAttribute('aria-label') ?? element.textContent ?? '').trim().slice(0, 40),
        // Either is a visible indicator. Asserting one particular mechanism
        // would fail the day the stylesheet lands and replaces the browser's
        // ring with its own.
        indicator: (style.outlineStyle !== 'none' && style.outlineWidth !== '0px') || style.boxShadow !== 'none',
      };
    });

    if (control === null) break;

    // A native date input is four Tab stops — day, month, year and the
    // picker — and only the first of them draws the ring on the host: the
    // rest are inside a shadow tree no script can read, so the host reports
    // no outline while the browser is quite plainly showing one. Collapsing
    // consecutive stops on the same identified element counts the control
    // once, which is also what "reachable" means to the person tabbing.
    const previous = walk[walk.length - 1];
    if (previous && control.id !== '' && previous.id === control.id && previous.tag === control.tag) {
      continue;
    }

    walk.push(control);
  }

  return walk;
}

// reached checks that every expected control appears in the walk, in order. A
// control that is missing is unreachable by keyboard; one out of order is a
// focus order that does not follow the page.
export function reached(walk: Focused[], expected: string[]): string[] {
  const seen = walk.map(describe);
  const remaining = [...expected];

  for (const control of seen) {
    if (remaining[0] === control) remaining.shift();
  }

  return remaining;
}
