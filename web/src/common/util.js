function GetBaseUrl() {
    return window.location.href.split("/").slice(0, 3).join("/");
}

export {
    GetBaseUrl
}
