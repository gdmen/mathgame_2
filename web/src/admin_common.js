// Shared building blocks for the admin report pages (difficulty-calibration and
// bitmap-matrix): they render stored expressions the same way, and poll the
// same way while a background rebuild runs.
import React, { useEffect } from "react";
import parse from "html-react-parser";
import katex from "katex";
import "katex/dist/katex.min.css";

import { PreprocessExpression } from "./problem.js";

// renderMath renders a stored expression to KaTeX the same way the play page
// does, so an admin view matches what kids actually see. Cached by expression
// string — the admin views re-render the same handful of expressions
// repeatedly (a virtualized grid re-renders them every scroll frame), and KaTeX
// parsing is the per-cell cost. Falls back to raw text if KaTeX can't parse it.
const mathCache = new Map();
const renderMath = (expr) => {
  if (!expr) {
    return null;
  }
  let html = mathCache.get(expr);
  if (html === undefined) {
    try {
      html = katex.renderToString(PreprocessExpression(expr));
    } catch (e) {
      html = null;
    }
    mathCache.set(expr, html);
  }
  return html === null ? <code>{expr}</code> : parse(html);
};

// usePollWhileComputing calls fn every 3s while computing is true, so a report
// (and its progress) refresh in place as a background rebuild lands.
const usePollWhileComputing = (computing, fn) => {
  useEffect(() => {
    if (!computing) {
      return undefined;
    }
    const t = setInterval(fn, 3000);
    return () => clearInterval(t);
  }, [computing, fn]);
};

export { renderMath, usePollWhileComputing };
