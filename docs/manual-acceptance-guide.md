# ORBIT manual accessibility acceptance — step-by-step

This guide is for a contributor who has never performed a manual accessibility review. It supplements the automated Chromium/Playwright/axe suite; it does not replace it. A pass here is evidence from the tested device, browser, and assistive technology only, not a WCAG certification.

## 1. Before starting

Use the stable v1.0.0 portfolio dashboard:

- https://pooya-labs.github.io/orbit/

No local checkout, terminal, GitHub permissions, API credentials, or production account is needed for this review. Use the visible demo/read-only mode. Do not configure a real API, submit operational mutations, enable notification channels, change alert rules, or enter secrets. If an action would create, update, or delete a real record, cancel it.

Record the environment before testing:

- Date and local time:
- Device and OS version:
- Browser and version:
- Assistive technology and version (or none for keyboard-only):
- ORBIT language and theme:
- Viewport / browser zoom:

Keep one browser tab for the review. Reload the page before each independent pass so focus starts from a known location.

## 2. Keyboard-only review (no screen reader required)

Use a physical keyboard. Once the page is loaded, do not use the mouse until the pass is finished.

### A. Skip link and primary navigation

1. Reload the page.
2. Press Tab once. The skip-to-main link should receive visible focus.
3. Press Enter. Focus or the viewport should move to the main content; it must not disappear behind the header.
4. Reload again. Press Tab to reveal the skip link, then press Tab repeatedly through the primary navigation.
5. Each focused navigation item must have a clearly visible focus indicator.
6. Press Enter on each primary view in turn. Confirm that the selected view changes, the active navigation item is visibly distinct, and focus is not lost.
7. Press Shift+Tab to move backwards. The order should be predictable; focus must not jump to the browser chrome unexpectedly.
8. Repeat after switching between dark and light themes.

### B. Controls, filters, charts, and tables

1. Continue with Tab through the current view.
2. Use Space or Enter on buttons and links. Use the arrow keys for native selects and radio-style controls.
3. Confirm that the focused control has a visible focus ring and that activation has an understandable result.
4. Change a chart time range, open its data-table alternative, and navigate the table with the keyboard.
5. Open and close native disclosure sections such as audit-event details. Confirm the disclosure state changes and focus remains sensible.
6. Check that disabled/unavailable preview controls cannot be activated and are visibly presented as unavailable.

Do not count a control as passing merely because it can receive focus: it must also have an understandable name, a predictable action, and a visible state change where applicable.

8. Activate **Settings** from the sidebar. Confirm the dialog opens, the close control receives focus, theme and language options update the live page, the selected option is announced, Escape closes the dialog, and focus returns to Settings. Use **Manage source** and verify the API source panel opens with its URL field focused.
9. Activate **Help & support**. Confirm the dialog opens, its project-documentation and support links have meaningful names and valid destinations, Escape and the close button both dismiss it, and focus returns to the Help control. Repeat both dialogs at a narrow mobile viewport and in Persian RTL.

### C. Forms and mobile navigation

1. On the Incidents, Alerts, Notifications, Deployments, and SLO views, tab through the visible fields and action buttons.
2. Confirm each field has a visible label and the keyboard can reach its associated input.
3. Check required-field feedback only where the form is clearly a local/demo interaction. Do not submit a real create/update/delete operation.
4. If the responsive mobile navigation drawer is available at the current viewport, open it using the keyboard.
5. Confirm focus moves into the drawer, Tab and Shift+Tab remain within it while open, Escape closes it, and focus returns to the drawer trigger.
6. Select a view from the drawer and confirm the drawer closes and focus remains usable.

### D. Reflow, language, and motion

1. Test both dark and light themes.
2. Test English LTR and Persian RTL.
3. Increase browser zoom with Ctrl and + to 200%, then 400%. The page must remain usable; text and controls must not overlap or become unreachable. Use Ctrl+0 to reset zoom.
4. At a narrow viewport, check that the page itself does not acquire unwanted horizontal scrolling. A chart's own labelled data/table scroll region may scroll horizontally.
5. If the operating system has a reduced-motion preference, enable it and reload. Long decorative transitions and smooth scrolling should be reduced.

Record the exact view and control whenever something fails. Do not guess which component is responsible.

## 3. Windows screen-reader review with NVDA

NVDA is free and open source. Download it only from the official NV Access site: https://www.nvaccess.org/download/

1. Download the current stable installer from NV Access.
2. Run the installer and follow its prompts. The optional desktop shortcut Ctrl+Alt+N can be enabled during setup; otherwise start NVDA from the Windows Start menu.
3. Open Microsoft Edge or Firefox and visit the ORBIT public preview.
4. Keep the default NVDA modifier key selected during setup. It is usually Insert (on some laptop layouts, Caps Lock can be selected).
5. Reload the page and listen to the initial announcement. It should identify the page/document rather than reading unrelated browser chrome as page content.
6. Press D in browse mode to move through landmarks. Confirm a primary navigation landmark and a main-content landmark are announced with useful names.
7. Press H to move through headings. Confirm each view has a meaningful heading and the heading changes after client-side navigation.
8. Press F to move among form fields, B among buttons, and K among links. Each control should be announced with a meaningful name, role, and state.
9. Press NVDA+F7 to inspect the Elements List. Check that links, headings, and form fields are discoverable and sensibly named.
10. Navigate to a chart and its expandable table. Confirm that the table is announced as a table, column headers make sense, and row/cell navigation exposes the relevant values.
11. Change to Persian RTL and repeat landmark, heading, navigation, and form-label checks. Check that the speech language/pronunciation is usable; record language-switching problems separately from missing labels.
12. Repeat the essential checks in the other browser (Edge or Firefox) when available.

Useful default NVDA commands:

| Action | Key |
| --- | --- |
| Next heading | H |
| Next landmark | D |
| Next form field | F |
| Next button | B |
| Next link | K |
| Elements List | NVDA+F7 |
| Stop current speech | Ctrl |
| NVDA menu | NVDA+N |
| Quit NVDA | NVDA+Q |

If single-letter navigation does not work, click the browser page once or press NVDA+Space to switch to browse mode, then retry. Do not install third-party NVDA add-ons for this review.

Official references:
- [NVDA User Guide](https://api.nvaccess.org/documentation/userGuide.html)
- [NV Access help and quick-start resources](https://www.nvaccess.org/get-help/)

## 4. Android screen-reader review with TalkBack

Use an Android phone and Chrome. The exact Settings labels can vary by manufacturer and Android version.

1. Open Android Settings.
2. Search Settings for TalkBack, or navigate to Accessibility → TalkBack.
3. Turn TalkBack on and follow the built-in tutorial before testing the website.
4. Open Chrome and visit the ORBIT public preview.
5. Swipe one finger right to move to the next accessible item; swipe left to move to the previous item.
6. Listen for the element's name, role, and state. Double-tap to activate the focused item.
7. Use a two-finger swipe to scroll the page. A one-finger swipe is used by TalkBack for navigation, not ordinary page scrolling.
8. Navigate the page from the beginning. Confirm the page title, navigation, active view, main content, headings, buttons, form fields, and chart-table alternative are announced in a sensible order.
9. Change between English/Persian and dark/light themes and repeat the essential checks.
10. Turn TalkBack off from the same Settings screen when finished.

If the first enablement is confusing, complete the built-in TalkBack tutorial first; gesture behavior and multi-finger support vary by device/version.

Official references:
- [Google: Get started with TalkBack](https://support.google.com/accessibility/android/answer/6283677)
- [Google: TalkBack gestures](https://support.google.com/accessibility/android/answer/6151827)
- [Samsung: Enable TalkBack on Galaxy devices](https://www.samsung.com/us/support/answer/ANS10002553/)

## 5. Apple VoiceOver review (when an Apple device is available)

VoiceOver is built into Apple devices. Do not mark this review as passed unless it was actually run on an Apple device.

- **iPhone/iPad:** Settings → Accessibility → VoiceOver → turn VoiceOver on. Complete the VoiceOver Practice/Tutorial first, then open the public preview in Safari. Swipe right/left to move between items and double-tap to activate the focused item.
- **Mac:** Press Command+F5, or open System Settings → Accessibility → VoiceOver. Open the public preview in Safari. The default VoiceOver modifier is Control+Option (or Caps Lock on supported setups).

Check the same landmarks, headings, labels, active-navigation state, forms, and chart-table structure as in the NVDA section. If no Apple device is available, record Not tested — no Apple device available; do not infer a pass from NVDA or TalkBack.

Official references:
- [Apple: Turn on and practice VoiceOver on iPhone](https://support.apple.com/en-euro/guide/iphone/iph3e2e415f/ios)
- [Apple: Turn VoiceOver on or off on Mac](https://support.apple.com/guide/voiceover/turn-voiceover-on-or-off/)

## 6. Visual and contrast review

Automated contrast checks and axe scans already cover defined states, but they do not sample every rendered color/background combination or replace visual inspection.

1. Inspect small/secondary text, disabled controls, badges, chart labels, table borders, and focus rings in both themes.
2. Check that status is not communicated by color alone.
3. Check hover and focus states separately.
4. Check the Persian RTL layout for clipped labels, reversed control order, and focus indicators hidden at the wrong edge.
5. If a color looks too faint, record the foreground and background if you can obtain them; do not change contrast thresholds or suppress axe rules to make the result pass.

## 7. What to report

For every finding, record:

- Result: Pass, Fail, or Not tested.
- View and exact control/heading.
- Steps to reproduce, starting from a page reload.
- Expected behavior.
- Actual visual result or exact spoken announcement.
- Device/OS, browser/version, screen-reader/version, language, theme, and viewport/zoom.
- Screenshot or short recording only if it contains no credentials or personal data.

Do not paste bearer tokens, API keys, cookies, webhook URLs, database URLs, private keys, or real customer telemetry into a report.

Use this template:

~~~text
Date/time:
Device / OS:
Browser / version:
Assistive technology / version:
Language / theme:
Viewport / zoom:
View / control:
Result (Pass / Fail / Not tested):
Steps:
Expected:
Actual / spoken announcement:
Evidence link (sanitized):
~~~

## 8. Acceptance boundary

A review result is valid only for the exact environment recorded. A successful NVDA pass on Windows does not imply VoiceOver or TalkBack passed. Keep every unavailable platform as Not tested. Once findings are recorded, fix reproducible defects and rerun the affected manual steps plus the automated CI suite. Keep the M01 gate open until the required human review is recorded in docs/accessibility-review.md.

This manual review does not, by itself, establish WCAG conformance or certification.
