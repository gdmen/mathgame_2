import React from "react";

const HomeView = ({ isLoading, isAuthenticated, user }) => {
  if (isLoading) {
    return (
      <div>loading</div>
    )
  }
  else if (!isAuthenticated) {
    return (
      <div>not logged in home info</div>
    )
  }
  else if (!user) {
    return (
      <div>loading</div>
    )
  }
  return (
    <div>
      {user.username}
    </div>
  )
}

export {
  HomeView
}
