import React from 'react'
import { useAuth0 } from "@auth0/auth0-react"

// TODO: add logout etc from
// https://auth0.com/docs/libraries/auth0-react#getting-started

const LoginButton = () => {
  const { loginWithRedirect } = useAuth0();

  return <button onClick={() => loginWithRedirect()}>Log In</button>;
};

export {
  LoginButton
}
