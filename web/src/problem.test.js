import katex from "katex";
import { PreprocessExpression } from "./problem.js";

describe("PreprocessExpression", () => {
  it("escapes a bare percent (KaTeX reads a bare % as a comment)", () => {
    expect(PreprocessExpression("50%")).toBe("\\text{50}\\%");
  });

  it("keeps division visible after a percent (regression: 70% ÷ 7)", () => {
    const out = PreprocessExpression("70% \\div 7");
    expect(out).toBe("\\text{70}\\% \\div 7");
    // Without the escape KaTeX would comment out " \div 7" and render just 70;
    // the rendered output must still carry the division sign.
    expect(katex.renderToString(out)).toContain("÷");
  });

  it("escapes every percent in the expression", () => {
    expect(PreprocessExpression("25% + 10%")).toBe(
      "\\text{25}\\% + \\text{10}\\%"
    );
  });

  it("does not double-escape an already-escaped percent", () => {
    expect(PreprocessExpression("\\%")).toBe("\\%");
  });
});
