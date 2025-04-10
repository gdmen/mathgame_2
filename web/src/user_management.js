import React, { useState } from "react";

import "./user_management.scss";

const postJSONForm = async function (url, body, setErrorFcn) {
  try {
      const reqParams = {
        method: "POST",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
        },
        body: JSON.stringify(body),
      };
      const req = await fetch(url, reqParams);
      const json = await req.json();
      setError(false);
    } catch (e) {
      console.log(e.message);
      setError(true);
    }
};

const EmailView = ({ url }) => {
  const [error, setError] = useState(true);
  const [email, setEmail] = useState("");

  const handleInputChange = (e) => {
    setEmail(e.target.value);
  }

  return (
    <>
      <div className="email-form">
        <input
          type="email"
          required="required"
          className={error ? "error" : ""}
          value={email}
          onChange={handleInputChange}
        />
        <button
          className={error ? "error" : ""}
          onClick={postJSONForm(url + "/auth/email", {email: email})}
        />
          sign up
        </button>
      </div>
    </>
  );
};

const PasswordView = ({ url, key }) => {
  const [error, setError] = useState(true);
  const [password, setPassword] = useState("");

  const handleInputChange = (e) => {
    setPassword(e.target.value);
  }

  return (
    <>
      <div className="password-form">
        <input
          type="text"
          minlength="8"
          required="required"
          className={error ? "error" : ""}
          value={password}
          onChange={handleInputChange}
        />
        <button
          className={error ? "error" : ""}
          onClick={postJSONForm(url + "/auth/password", {password: password, key: key}, setError)}
        />
          sign up
        </button>
      </div>
    </>
  );
};
