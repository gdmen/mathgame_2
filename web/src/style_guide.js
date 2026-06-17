import React, { useState } from "react";
import PinInput from "react-pin-input";

import "./style_guide.scss";
// Import the SCSS files for the compound components we render below, so
// every example uses the same styles users actually see. Adding a new
// compound here means adding the corresponding @import.
import "./setup.scss";
import "./settings.scss";
import "./pin.scss";
import "./play.scss";
import "./progress.scss";
import "./home.scss";

// Design tokens — keep in sync with styles.scss. Labels are the static
// reference; rendered examples below use the actual SCSS variables, so a
// mismatch between label and rendered look means this list drifted.
const COLORS = [
  { name: "color-one", hex: "#bee8b7", note: "primary brand" },
  { name: "color-two", hex: "#d6f7d2", note: "primary brand, lighter" },
  { name: "color-one-contrast", hex: "#007200", note: "contrast on color-one" },
  { name: "color-inactive", hex: "#a9a9a9", note: "disabled / muted" },
  { name: "color-error", hex: "#dc143c", note: "errors, destructive" },
  {
    name: "color-card-tint-a",
    hex: "#f3faf3",
    note: "problem-type card surface A",
  },
  {
    name: "color-card-tint-b",
    hex: "#f7faf5",
    note: "problem-type card surface B",
  },
  {
    name: "color-chip-border",
    hex: "#c9d2c7",
    note: "toggle chip / card borders",
  },
  { name: "background-color", hex: "#ffffff", note: "page background" },
  { name: "font-color", hex: "#000000", note: "default text" },
];

// Used in production but not assigned to an SCSS variable.
const UNTOKENIZED_COLORS = [
  {
    name: "whitesmoke",
    hex: "#f5f5f5",
    note: "subtle row backgrounds, hover states",
  },
  {
    name: "lightgray",
    hex: "#d3d3d3",
    note: "disabled-state button background, dotted borders",
  },
  { name: "gray", hex: "#808080", note: "settings-hint text, generic borders" },
];

// Weights actually in use across the site. The base reset sets 300 for
// p/button; everything else is hand-applied.
const FONT_WEIGHTS = [
  { value: "300", note: "body, buttons (default reset)" },
  { value: "400", note: "settings/home/setup buttons" },
  { value: "600", note: "progress-summary value, table headers" },
  { value: "bold", note: "rare; setup strong tags" },
];

// Below the base 1em sit a few recurring small-text sizes.
const SMALL_TEXT = [
  { value: "0.95em", note: "report-modal copy + error text" },
  { value: "0.9em", note: ".settings-hint, report-problem-link, sub-headings" },
  {
    value: "0.875rem",
    note: ".progress-by-month-table, .progress-summary-label",
  },
  { value: "0.85em", note: ".report-char-count, .report-modal-actions" },
];

const SPACING = [
  { name: "base-space", value: "1em", note: "default padding / gap unit" },
  { name: "max-width", value: "1200px", note: "content max width" },
];

const SHAPE = [
  {
    name: "base-radius",
    value: "0.5em",
    note: "default corner radius (token)",
  },
  {
    name: "small radius",
    value: "0.25em",
    note: "pills, pin inputs, modal/textarea corners (not tokenized — used inline)",
  },
];

const BREAKPOINTS = [
  {
    name: "Mobile (home)",
    value: "max-width: 768px",
    note: "hero collapses; image hidden",
  },
  {
    name: "Mobile (setup/settings)",
    value: "max-width: 870px",
    note: "tab labels hidden, form widens to 100%",
  },
];

const Section = ({ title, children }) => (
  <section className="sg-section">
    <h2>{title}</h2>
    {children}
  </section>
);

const TokenRow = ({ swatch, name, value, note }) => (
  <div className="sg-token-row">
    <div className="sg-swatch">{swatch}</div>
    <div className="sg-token-meta">
      <code className="sg-token-name">{name}</code>
      <code className="sg-token-value">{value}</code>
      {note && <span className="sg-token-note">{note}</span>}
    </div>
  </div>
);

// Wraps a compound example with its class-chain note so the reference
// makes the parent context required by the source SCSS explicit.
const CompoundExample = ({ name, context, children }) => (
  <div className="sg-compound">
    <div className="sg-compound-meta">
      <code className="sg-compound-name">{name}</code>
      {context && <code className="sg-compound-context">{context}</code>}
    </div>
    <div className="sg-compound-frame">{children}</div>
  </div>
);

const StyleGuideView = () => {
  // PinInput needs state to render its filled/unfilled segments cleanly.
  const [demoPin, setDemoPin] = useState("");
  // Modal demo state.
  const [showModal, setShowModal] = useState(false);

  return (
    <div className="sg-page">
      <header className="sg-header">
        <h1>Style Guide</h1>
        <p>
          Live reference for the design tokens and components used across the
          site. Token values come from <code>web/src/styles.scss</code>.
          Compound components are rendered using the production SCSS, so what
          you see here is what users see.
        </p>
        <p>
          Scope is the design system as of commit <code>930927d</code> (the last
          commit before LLM-driven UI additions started landing without
          referencing this guide). Newer UI may introduce patterns that
          aren&rsquo;t documented here on purpose — the goal is for this page to
          define what&rsquo;s canonical, not to ratify whatever shipped most
          recently.
        </p>
      </header>

      <Section title="Colors">
        <div className="sg-grid">
          {COLORS.map((c) => (
            <TokenRow
              key={c.name}
              swatch={
                <div
                  className="sg-color-swatch"
                  style={{ backgroundColor: c.hex }}
                />
              }
              name={"$" + c.name}
              value={c.hex}
              note={c.note}
            />
          ))}
        </div>
        <h3>Untokenized but used</h3>
        <p>
          These show up in production CSS as bare named colors. Worth knowing
          about, worth not inventing more of them.
        </p>
        <div className="sg-grid">
          {UNTOKENIZED_COLORS.map((c) => (
            <TokenRow
              key={c.name}
              swatch={
                <div
                  className="sg-color-swatch"
                  style={{ backgroundColor: c.hex }}
                />
              }
              name={c.name}
              value={c.hex}
              note={c.note}
            />
          ))}
        </div>
      </Section>

      <Section title="Typography">
        <p>
          Font family: <code>Josefin Sans, sans-serif</code> (loaded from Google
          Fonts).
        </p>
        <div className="sg-type-scale">
          <div className="sg-type-row">
            <h1>Heading 1 / 3em</h1>
            <code>h1</code>
          </div>
          <div className="sg-type-row">
            <h2>Heading 2 / 2em</h2>
            <code>h2</code>
          </div>
          <div className="sg-type-row">
            <h3>Heading 3 / 1.5em</h3>
            <code>h3</code>
          </div>
          <div className="sg-type-row">
            <h4>Heading 4 / UA default</h4>
            <code>h4</code>
          </div>
          <div className="sg-type-row">
            <h5>Heading 5 / UA default</h5>
            <code>h5</code>
          </div>
          <div className="sg-type-row">
            <p>
              Body paragraph at base size (1em). Headings get line-height
              1.25em.
            </p>
            <code>p</code>
          </div>
        </div>

        <h3>Font weights</h3>
        <p>
          The base reset sets weight 300 on <code>p</code> and{" "}
          <code>button</code>. Heavier weights are applied per-component.
        </p>
        <div className="sg-type-scale">
          {FONT_WEIGHTS.map((w) => (
            <div className="sg-type-row" key={w.value}>
              <span style={{ fontWeight: w.value }}>
                The quick brown fox — weight {w.value}
              </span>
              <code>{w.note}</code>
            </div>
          ))}
        </div>

        <h3>Small text</h3>
        <p>Below 1em sit two recurring sizes used for hints and metadata.</p>
        <div className="sg-type-scale">
          {SMALL_TEXT.map((s) => (
            <div className="sg-type-row" key={s.value}>
              <span style={{ fontSize: s.value }}>Sample at {s.value}</span>
              <code>{s.note}</code>
            </div>
          ))}
        </div>
      </Section>

      <Section title="Links">
        <p>
          In-text:{" "}
          <a href="#" onClick={(e) => e.preventDefault()}>
            anchor color
          </a>{" "}
          (browser default — no global anchor color is set).
        </p>
        <p className="sg-footer-link-demo">
          Footer style (inherit color, no underline):{" "}
          <a href="#" onClick={(e) => e.preventDefault()}>
            report an issue
          </a>{" "}
          <span className="separator">|</span>{" "}
          <a href="#" onClick={(e) => e.preventDefault()}>
            source code
          </a>
        </p>
      </Section>

      <Section title="Spacing">
        <div className="sg-grid">
          {SPACING.map((s) => (
            <TokenRow
              key={s.name}
              swatch={
                <div
                  className="sg-space-swatch"
                  style={
                    s.name === "base-space"
                      ? { width: "1em", height: "1em" }
                      : { width: "100%", height: "0.5em" }
                  }
                />
              }
              name={"$" + s.name}
              value={s.value}
              note={s.note}
            />
          ))}
        </div>
      </Section>

      <Section title="Shape">
        <div className="sg-grid">
          {SHAPE.map((s) => (
            <TokenRow
              key={s.name}
              swatch={
                <div
                  className="sg-shape-swatch"
                  style={{ borderRadius: s.value }}
                />
              }
              name={s.name.startsWith("base") ? "$" + s.name : s.name}
              value={s.value}
              note={s.note}
            />
          ))}
        </div>
      </Section>

      <Section title="Breakpoints">
        <div className="sg-grid">
          {BREAKPOINTS.map((b) => (
            <TokenRow
              key={b.name}
              swatch={<div className="sg-bp-swatch" />}
              name={b.name}
              value={"@media (" + b.value + ")"}
              note={b.note}
            />
          ))}
        </div>
      </Section>

      <Section title="Buttons">
        <p>
          The bare <code>&lt;button&gt;</code> reset sets a white background, no
          border, base-radius corners, and base-space padding. The{" "}
          <code>.submit</code> variant (used by setup and settings forms)
          inverts: green background, white text. Its <code>.error</code> state
          mutes to lightgray and disables interaction.
        </p>
        <div className="sg-button-row">
          <button>default</button>
          <button disabled>disabled (UA default)</button>
        </div>
        <CompoundExample
          name=".submit"
          context=".settings .tab-content button.submit"
        >
          <div className="settings">
            <div className="tab-content sg-no-padding">
              <button className="submit">Submit</button>
              <button className="submit error">Submit (error)</button>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="Form elements">
        <div className="sg-form-row">
          <label>
            Text input
            <input type="text" placeholder="placeholder" />
          </label>
          <label>
            Number input
            <input type="number" placeholder="0" />
          </label>
          <label>
            <input type="checkbox" defaultChecked /> Checkbox
          </label>
          <label>
            Select
            <select>
              <option>Option A</option>
              <option>Option B</option>
            </select>
          </label>
        </div>
      </Section>

      <Section title="Form layouts">
        <CompoundExample
          name=".setup-form"
          context="inline-block, 80% width, used inside #setup .tab-content"
        >
          <div id="setup">
            <div className="tab-content sg-no-padding">
              <div className="setup-form">
                <h4>
                  Field heading <span className="error">(error hint)</span>
                </h4>
                <input type="text" placeholder="field" />
              </div>
            </div>
          </div>
        </CompoundExample>
        <CompoundExample
          name=".settings-form + .settings-hint"
          context=".settings .tab-content"
        >
          <div className="settings">
            <div className="tab-content sg-no-padding">
              <div className="settings-form">
                <h4>Field heading</h4>
                <div className="settings-hint">
                  Help text — gray, slightly smaller.
                </div>
                <input type="text" placeholder="value" />
              </div>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="Numbered step tabs">
        <p>
          The multi-step setup flow. Active tab gets a colored border-bottom and
          a colored circular number.
        </p>
        <CompoundExample
          name="#setup-tabs"
          context="div#setup > #setup-tabs > .tab[.active]"
        >
          <div id="setup">
            <div id="setup-tabs">
              <div className="tab active">
                <span className="number">1</span>
                <span className="label">Active step</span>
              </div>
              <div className="tab">
                <span className="number">2</span>
                <span className="label">Pending step</span>
              </div>
              <div className="tab">
                <span className="number">3</span>
                <span className="label">Pending step</span>
              </div>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="Circular badge">
        <p>The numbered badge primitive from setup tabs, usable on its own.</p>
        <CompoundExample
          name=".number"
          context="#setup-tabs .tab .number (inactive) / .tab.active .number (active)"
        >
          <div id="setup">
            <div id="setup-tabs sg-inline-badges">
              <div className="tab">
                <span className="number">1</span>
              </div>
              <div className="tab active">
                <span className="number">2</span>
              </div>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="Problem-type toggle pills">
        <p>
          Topic toggles in settings. A hidden checkbox drives a colored
          background on its sibling pill. Issue{" "}
          <a
            href="https://github.com/gdmen/mathgame_2/issues/225"
            target="_blank"
            rel="noopener noreferrer"
          >
            #225
          </a>{" "}
          extends this pattern to many more bits.
        </p>
        <CompoundExample
          name=".problem-type-button"
          context=".settings .tab-content #problem-types-settings #problem-type-buttons li label"
        >
          <div className="settings">
            <div className="tab-content sg-no-padding">
              <div id="problem-types-settings">
                <ul id="problem-type-buttons">
                  <li>
                    <input
                      type="checkbox"
                      id="sg-pt-add"
                      defaultChecked
                      readOnly
                    />
                    <label htmlFor="sg-pt-add">
                      <span className="problem-type-button">Addition</span>
                    </label>
                  </li>
                  <li>
                    <input type="checkbox" id="sg-pt-sub" readOnly />
                    <label htmlFor="sg-pt-sub">
                      <span className="problem-type-button">Subtraction</span>
                    </label>
                  </li>
                  <li>
                    <input
                      type="checkbox"
                      id="sg-pt-mul"
                      defaultChecked
                      readOnly
                    />
                    <label htmlFor="sg-pt-mul">
                      <span className="problem-type-button">
                        Multiplication
                      </span>
                    </label>
                  </li>
                </ul>
              </div>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="PIN input">
        <p>
          Four-digit PIN entry using <code>react-pin-input</code>. Used in the
          parent-gate pin screen and in the report-problem / delete-account
          modals. The shared modal extraction is tracked in{" "}
          <a
            href="https://github.com/gdmen/mathgame_2/issues/217"
            target="_blank"
            rel="noopener noreferrer"
          >
            #217
          </a>
          .
        </p>
        <CompoundExample
          name="PinInput"
          context='length=4, inputStyle={{ borderRadius: "0.25em" }}'
        >
          <div className="pin-form sg-pin-frame">
            <PinInput
              length={4}
              initialValue={demoPin}
              onChange={(value) => setDemoPin(value)}
              onComplete={() => {}}
              type="numeric"
              inputMode="numeric"
              inputStyle={{ borderRadius: "0.25em" }}
            />
          </div>
        </CompoundExample>
      </Section>

      <Section title="Modal">
        <p>
          Centered overlay card. Used today in the report-problem flow (
          <code>.report-modal</code>) and the delete-account flow. Will collapse
          into a single shared component per{" "}
          <a
            href="https://github.com/gdmen/mathgame_2/issues/217"
            target="_blank"
            rel="noopener noreferrer"
          >
            #217
          </a>
          .
        </p>
        <button
          className="sg-modal-open-btn"
          onClick={() => setShowModal(true)}
        >
          Open demo modal
        </button>
        {showModal && (
          <>
            <div
              className="report-modal-overlay"
              onClick={() => setShowModal(false)}
            />
            <div className="report-modal" onClick={(e) => e.stopPropagation()}>
              <h4>Confirm action</h4>
              <p className="report-modal-copy">
                Demonstration of the modal shape: centered card with shadow,
                overlay closes on outside click.
              </p>
              <div className="report-modal-pin">
                <label>Enter PIN to confirm</label>
                <PinInput
                  length={4}
                  type="numeric"
                  inputMode="numeric"
                  inputStyle={{ borderRadius: "0.25em" }}
                  onChange={() => {}}
                  onComplete={() => {}}
                />
              </div>
              <div className="report-modal-actions">
                <button onClick={() => setShowModal(false)}>Cancel</button>
                <button onClick={() => setShowModal(false)}>Confirm</button>
              </div>
            </div>
          </>
        )}
      </Section>

      <Section title="Landing hero">
        <p>
          The homepage hero band. Color-one background, two-column flex layout,
          primary CTA in the contrast color. On <code>max-width: 768px</code>{" "}
          the image hides and the copy goes full-width.
        </p>
        <CompoundExample
          name="#landing-hero"
          context="renders inside #content; full-bleed bg"
        >
          <div id="landing-hero">
            <div className="hero-content">
              <div className="hero-copy">
                <h2>Headline copy goes here</h2>
                <p>
                  Supporting paragraph at 1.125em, line-height 1.5, with a
                  primary CTA below.
                </p>
                <div className="button-container">
                  <button>Get started</button>
                </div>
              </div>
              <div className="hero-image sg-hero-placeholder" />
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="Stat cards">
        <p>
          The progress page's summary tiles. Big value, small label, light gray
          block.
        </p>
        <CompoundExample
          name=".progress-summary-item"
          context=".progress-page .progress-summary > .progress-summary-item"
        >
          <div className="progress-page sg-no-padding">
            <div className="progress-summary">
              <div className="progress-summary-item">
                <div className="progress-summary-value">123</div>
                <div className="progress-summary-label">problems solved</div>
              </div>
              <div className="progress-summary-item">
                <div className="progress-summary-value">12h 4m</div>
                <div className="progress-summary-label">time played</div>
              </div>
              <div className="progress-summary-item">
                <div className="progress-summary-value">7</div>
                <div className="progress-summary-label">day streak</div>
              </div>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="Data table">
        <p>The only table pattern on the site (progress page).</p>
        <CompoundExample
          name=".progress-by-month-table"
          context=".progress-page .progress-by-month > table"
        >
          <div className="progress-page sg-no-padding">
            <div className="progress-by-month">
              <table className="progress-by-month-table">
                <thead>
                  <tr>
                    <th>Month</th>
                    <th>Solved</th>
                    <th>Time</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td>April</td>
                    <td>84</td>
                    <td>3h 22m</td>
                  </tr>
                  <tr>
                    <td>May</td>
                    <td>112</td>
                    <td>4h 17m</td>
                  </tr>
                  <tr>
                    <td>June</td>
                    <td>56</td>
                    <td>2h 5m</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="Playlist list item">
        <p>
          Row layout for playlists and videos: optional thumbnail (16:9), title
          (ellipsizes), remove/action cell on the right.
        </p>
        <CompoundExample
          name=".playlist-item"
          context=".settings .tab-content #playlists-settings ul#playlist-list > .playlist-item"
        >
          <div className="settings">
            <div className="tab-content sg-no-padding">
              <div id="playlists-settings">
                <ul id="playlist-list">
                  <li className="playlist-list-header">
                    <span style={{ marginLeft: "0.5em" }}>Title</span>
                  </li>
                  <li className="playlist-item">
                    <div className="playlist-thumbnail sg-thumb-placeholder" />
                    <a
                      href="#"
                      className="playlist-title"
                      onClick={(e) => e.preventDefault()}
                    >
                      Sample playlist title that may overflow if very long
                    </a>
                    <span className="playlist-remove">×</span>
                  </li>
                </ul>
              </div>
            </div>
          </div>
        </CompoundExample>
      </Section>

      <Section title="States">
        <p>The site's stock loading state:</p>
        <div className="content-loading" />
      </Section>

      <footer className="sg-footer">
        <p>
          This page lives at <code>/admin/style-guide</code> and is linked from
          the admin home (admin only).
        </p>
      </footer>
    </div>
  );
};

export { StyleGuideView };
