import React from 'react'
import { useAuth0 } from "@auth0/auth0-react"


class LoginView extends React.Component {
  render() {
    return (
      <div>
        <LoginButton />
        <br />
        This should have a login form + request a JWT? + store the JWT client side?
      </div>
    )
  }
}

export {
  LoginView
}
