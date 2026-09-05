// T097, discharging SC-002. The active-patient pointer, driven the way a
// person actually drives it: choose somebody in the switcher, watch three
// screens each name them, reload, and find the choice still there.
//
// This case registers a fresh account rather than using one of the three
// seeded ones (recovery.spec.ts's own reason: the three are shared by every
// other case in the run, and this one needs an account with nobody's history
// on it and twenty-five patients of its own, for SC-002's median-of-five
// timing).
import { randomUUID } from 'node:crypto';

import { fixtures } from './fixtures';
import { open } from './gate';
import { expect, test } from './auth';

// internal/web/page/patients.go's two routes and internal/httproute/routes.go's
// `base + "/active-patient"` (base is `/api/v1/me`). Spelled directly rather
// than read out of the Go source the way fixtures.ts does: this file needs
// three addresses and fixtures.ts already asserts the shell of every one of
// them elsewhere, so a rename here fails on a 404 rather than silently.
const patientsListPath = '/patients';
const patientDetailPath = (id: string) => `/patients/${id}`;

type Patient = { id: string; first_name: string; last_name: string };

async function newAccount(page: import('@playwright/test').Page) {
  const email = `patient-switch-${randomUUID().slice(0, 8)}@example.test`;
  const password = fixtures.password;

  const registered = await page.request.post(fixtures.registerPath, {
    data: { email, name: 'Patient Switch Rehearsal', password },
  });
  expect(registered.status(), await registered.text()).toBe(201);

  return { email, password };
}

async function createPatients(page: import('@playwright/test').Page, count: number): Promise<Patient[]> {
  const patients: Patient[] = [];

  for (let i = 0; i < count; i += 1) {
    // "self" is off limits: registration already provisioned the account's
    // own self-record (FR-004), and a second one is refused by idx_patients_self.
    const response = await page.request.post('/api/v1/patients', {
      data: {
        first_name: `Person${i}`,
        last_name: 'Rehearsal',
        birth_date: `19${70 + (i % 25)}-0${(i % 9) + 1}-1${(i % 9) + 1}`,
        relationship_to_owner: 'other',
      },
    });
    expect(response.ok(), `creating patient ${i} failed: ${await response.text()}`).toBe(true);
    patients.push((await response.json()) as Patient);
  }

  return patients;
}

async function switchTo(page: import('@playwright/test').Page, patientID: string): Promise<void> {
  const switcher = page.getByRole('combobox', { name: 'Active patient' });
  // Two interactions, counted from the accessibility tree rather than a fixed
  // selector: open the combobox, choose the option. selectOption drives both
  // through the accessibility tree in one Playwright action, which is what
  // "no more than two interactions" means for a native control.
  const switched = page.waitForResponse(
    (response) => response.request().method() === 'PUT' && response.url().includes('/api/v1/me/active-patient'),
  );
  await switcher.selectOption(patientID);
  expect((await switched).ok(), 'the switch was refused').toBe(true);
  await expect(switcher).toHaveValue(patientID);
}

test.describe('the active-patient switcher', () => {
  test.use({ signedInAs: 'anonymous' });

  test('names the person in view across three screens, and the choice survives a reload', async ({ page }) => {
    await newAccount(page);
    const patients = await createPatients(page, 3);
    const target = patients[1];
    const targetName = `${target.first_name} ${target.last_name}`;

    await open(page, {
      path: patientsListPath,
      title: fixtures.title('People'),
      landmark: { role: 'region', name: 'Patients' },
    });

    await switchTo(page, target.id);
    await expect(page.getByRole('link', { name: targetName })).toBeVisible();

    // Screen 2: the patient's own detail page.
    await page.goto(patientDetailPath(target.id));
    await expect(page.getByRole('heading', { name: new RegExp(targetName) })).toBeVisible();

    // Screen 3: the medications list, opened bare — the pointer, not a
    // query parameter this test supplied, is what scopes it.
    await page.goto('/medications');
    await expect(page).toHaveURL(new RegExp(`patient=${target.id}`));
    await expect(page.getByRole('link', { name: targetName })).toBeVisible();

    // Reload and the choice is still there — SC-002's second sentence,
    // "at both viewports" (this spec runs once per project, per
    // playwright.config.ts, exactly like smoke.spec.ts).
    await page.reload();
    await expect(page.getByRole('combobox', { name: 'Active patient' })).toHaveValue(target.id);
    await expect(page.getByRole('link', { name: targetName })).toBeVisible();
  });

  test('reaches any of twenty-five people in one switch, with a median under one second (SC-002)', async ({ page }) => {
    await newAccount(page);
    const patients = await createPatients(page, 25);

    await page.goto(patientsListPath);

    const samples: number[] = [];

    for (let i = 0; i < 5; i += 1) {
      const target = patients[(i * 5) % patients.length];
      const targetName = `${target.first_name} ${target.last_name}`;

      const started = Date.now();
      await switchTo(page, target.id);
      await expect(page.getByRole('link', { name: targetName })).toBeVisible();
      samples.push(Date.now() - started);
    }

    samples.sort((a, b) => a - b);
    const median = samples[Math.floor(samples.length / 2)];

    expect(median, `median switch time ${median}ms across ${JSON.stringify(samples)}`).toBeLessThanOrEqual(1000);
  });
});
