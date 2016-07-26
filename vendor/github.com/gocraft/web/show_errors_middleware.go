package web

import (
	"bufio"
	"html/template"
	"net/http"
	"os"
	"runtime"
	"strings"
)

// ShowErrorsMiddleware will catch panics and render an HTML page with the stack trace.
// This middleware should only be used in development.
func ShowErrorsMiddleware(rw ResponseWriter, req *Request, next NextMiddlewareFunc) {
	defer func() {
		if err := recover(); err != nil {
			const size = 4096
			stack := make([]byte, size)
			stack = stack[:runtime.Stack(stack, false)]

			renderPrettyError(rw, req, err, stack)
		}
	}()

	next(rw, req)
}

func renderPrettyError(rw ResponseWriter, req *Request, err interface{}, stack []byte) {
	_, filePath, line, _ := runtime.Caller(5)

	data := map[string]interface{}{
		"Error":    err,
		"Stack":    string(stack),
		"Params":   req.URL.Query(),
		"Method":   req.Method,
		"FilePath": filePath,
		"Line":     line,
		"Lines":    readErrorFileLines(filePath, line),
	}

	rw.Header().Set("Content-Type", "text/html")
	rw.WriteHeader(http.StatusInternalServerError)

	tpl := template.Must(template.New("ErrorPage").Parse(panicPageTpl))
	tpl.Execute(rw, data)
}

func readErrorFileLines(filePath string, errorLine int) map[int]string {
	lines := make(map[int]string)

	file, err := os.Open(filePath)
	if err != nil {
		return lines
	}

	defer file.Close()

	reader := bufio.NewReader(file)
	currentLine := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil || currentLine > errorLine+5 {
			break
		}

		currentLine++

		if currentLine >= errorLine-5 {
			lines[currentLine] = strings.Replace(line, "\n", "", -1)
		}
	}

	return lines
}

const panicPageTpl string = `
  <html>
    <head>
      <title>Panic</title>
      <meta http-equiv="Content-Type" content="text/html; charset=utf-8">
      <style>
      html, body{ padding: 0; margin: 0; }
      header { background: #C52F24; color: white; border-bottom: 2px solid #9C0606; }
      h1 { padding: 10px 0; margin: 0; }
      .container { margin: 0 20px; }
      .error { font-size: 18px; background: #FFCCCC; color: #9C0606; padding: 10px 0; }
      .file-info .file-name { font-weight: bold; }
      .stack { height: 300px; overflow-y: scroll; border: 1px solid #e5e5e5; padding: 10px; }

      table.source {
        width: 100%;
        border-collapse: collapse;
        border: 1px solid #e5e5e5;
      }

      table.source td {
        padding: 0;
      }

      table.source .numbers {
        font-size: 14px;
        vertical-align: top;
        width: 1%;
        color: rgba(0,0,0,0.3);
        text-align: right;
      }

      table.source .numbers .number {
        display: block;
        padding: 0 5px;
        border-right: 1px solid #e5e5e5;
      }

      table.source .numbers .number.line-{{ .Line }} {
        border-right: 1px solid #ffcccc;
      }

      table.source .numbers pre {
        white-space: pre-wrap;
      }

      table.source .code {
        font-size: 14px;
        vertical-align: top;
      }

      table.source .code .line {
        padding-left: 10px;
        display: block;
      }

      table.source .numbers .number,
      table.source .code .line {
        padding-top: 1px;
        padding-bottom: 1px;
      }

      table.source .code .line:hover {
        background-color: #f6f6f6;
      }

      table.source .line-{{ .Line }},
      table.source line-{{ .Line }},
      table.source .code .line.line-{{ .Line }}:hover {
        background: #ffcccc;
      }
      </style>
    </head>
  <body>
    <header>
      <div class="container">
        <h1>Error</h1>
      </div>
    </header>

    <div class="error">
      <p class="container">{{ .Error }}</p>
    </div>

    <div class="container">
      <p class="file-info">
        In <span class="file-name">{{ .FilePath }}:{{ .Line }}</span></p>
      </p>

      <table class="source">
        <tr>
          <td class="numbers">
            <pre>{{ range $lineNumber, $line :=  .Lines }}<span class="number line-{{ $lineNumber }}">{{ $lineNumber }}</span>{{ end }}</pre>
          </td>
          <td class="code">
            <pre>{{ range $lineNumber, $line :=  .Lines }}<span class="line line-{{ $lineNumber }}">{{ $line }}<br /></span>{{ end }}</pre>
          </td>
        </tr>
      </table>
      <h2>Stack</h2>
      <pre class="stack">{{ .Stack }}</pre>
      <h2>Request</h2>
      <p><strong>Method:</strong> {{ .Method }}</p>
      <h3>Paramenters:</h3>
      <ul>
        {{ range $key, $value := .Params }}
          <li><strong>{{ $key }}:</strong> {{ $value }}</li>
        {{ end }}
      </ul>
    </div>
  </body>
  </html>
  `
