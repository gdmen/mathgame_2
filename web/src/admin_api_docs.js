import React, { useEffect, useRef, useState } from "react";

import "./admin_api_docs.scss";

// Swagger UI is by far the largest thing the app ships and only this page uses
// it, so it is loaded on demand rather than bundled with the shell.
const loadSwaggerUI = () =>
  Promise.all([
    import("swagger-ui-dist/swagger-ui-bundle.js"),
    import("swagger-ui-dist/swagger-ui.css"),
  ]).then(([m]) => m.default);

// ApiDocsView renders the Swagger UI over the spec apiserver serves at
// /admin/swagger.yaml; see docs/swagger.md.
const ApiDocsView = ({ token, apiUrl }) => {
  const domRef = useRef(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!token || !apiUrl) {
      return undefined;
    }
    let cancelled = false;
    setError(null);
    loadSwaggerUI()
      .then((SwaggerUIBundle) => {
        if (cancelled || !domRef.current) {
          return;
        }
        SwaggerUIBundle({
          domNode: domRef.current,
          url: apiUrl + "/admin/swagger.yaml",
          // The default badge would send the admin-gated spec URL to
          // validator.swagger.io, which cannot read it anyway.
          validatorUrl: null,
          requestInterceptor: (req) => {
            req.headers.Authorization = "Bearer " + token;
            return req;
          },
        });
      })
      .catch((e) => {
        if (!cancelled) {
          setError(e.message || "Could not load the API docs");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [token, apiUrl]);

  return (
    <div className="api-docs-page">
      {error ? <span className="error">{error}</span> : null}
      <div ref={domRef} />
    </div>
  );
};

export { ApiDocsView };
