package server

import (
	"fmt"
	"net/http"
)

// playgroundHandler returns the GraphQL Playground HTML, pointed at the
// given endpoint. Inlined here (rather than importing the
// 99designs/gqlgen/graphql/playground package) to keep gocore/server
// thin — playground is a dev-only convenience and a single HTML page.
func playgroundHandler(endpoint string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, playgroundHTML, endpoint)
	})
}

const playgroundHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>GraphQL Playground</title>
  <link rel="stylesheet" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
  <link rel="shortcut icon" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
  <script src="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>
<body>
  <div id="root"></div>
  <script>
    window.addEventListener('load', function () {
      GraphQLPlayground.init(document.getElementById('root'), {
        endpoint: %q,
      });
    });
  </script>
</body>
</html>`
