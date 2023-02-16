import React from 'react'
import { useAuth0 } from "@auth0/auth0-react"

const LoginButton = () => {
  const { loginWithRedirect } = useAuth0();

  return <button className="login" onClick={() => loginWithRedirect()}>Log In</button>;
};

const SignupButton = () => {
  const { loginWithRedirect } = useAuth0();

  return <button className="signup" onClick={() => loginWithRedirect()}><h3>Get Started!</h3></button>;
};

const LogoutButton = () => {
  const { logout } = useAuth0();

  return (
    <button className="logout" onClick={() => {
      logout({ returnTo: window.location.origin });
    }}>Log Out</button>
  )
};

export {
  LoginButton,
  SignupButton,
  LogoutButton
}
