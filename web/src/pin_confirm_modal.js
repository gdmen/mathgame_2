import React, { useState } from "react";
import PinInput from "react-pin-input";

import { PIN_DIGIT_LABEL, PIN_LENGTH } from "./pin.js";

// The PIN re-entry gate an adult passes before an action a kid must not be able
// to trigger: reporting a problem, deleting the account. It owns the typed PIN
// and hands it to onConfirm, so a caller never holds a half-entered code and a
// fresh mount is what clears it.
//
// Confirm stays disabled until all four digits are in — a PIN gate has nothing
// to check before that. Extra fields (the report flow's explanation) render as
// children, between the PIN and the error line.
//
// The digits are masked: an adult types this with a kid watching the same
// screen, and a shoulder-surfed PIN is a defeated gate.
const PinConfirmModal = ({
  title,
  copy,
  pinLabel = "Enter PIN to confirm",
  confirmLabel,
  submittingLabel,
  confirmClassName,
  submitting = false,
  error,
  onConfirm,
  onCancel,
  children,
}) => {
  const [pin, setPin] = useState("");

  // A submit in flight is not cancellable: the request is already gone, and the
  // delete flow's success path is a logout.
  const dismiss = () => {
    if (!submitting) {
      onCancel();
    }
  };

  return (
    <div className="pin-confirm-modal-overlay" onClick={dismiss}>
      <div className="pin-confirm-modal" onClick={(e) => e.stopPropagation()}>
        <h4>{title}</h4>
        <p className="pin-confirm-modal-copy">{copy}</p>
        <div className="pin-confirm-modal-pin">
          <label>{pinLabel}</label>
          <PinInput
            focus
            length={PIN_LENGTH}
            type="numeric"
            inputMode="numeric"
            secret
            ariaLabel={PIN_DIGIT_LABEL}
            inputStyle={{ borderRadius: "0.25em" }}
            onChange={(value) => setPin(value)}
            onComplete={() => {}}
          />
        </div>
        {children}
        {error && <p className="pin-confirm-modal-error">{error}</p>}
        <div className="pin-confirm-modal-actions">
          <button type="button" onClick={dismiss} disabled={submitting}>
            Cancel
          </button>
          <button
            type="button"
            className={confirmClassName}
            onClick={() => onConfirm(pin)}
            disabled={submitting || pin.length < PIN_LENGTH}
            aria-busy={submitting}
          >
            {submitting ? submittingLabel : confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
};

export { PinConfirmModal };
