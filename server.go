// server
package main

import (
	//	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http/httputil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	//	"html/template"

	"github.com/kardianos/osext"

	_ "github.com/denisenkom/go-mssqldb"
	"github.com/kataras/iris"
	"github.com/kataras/iris/context"
	irisLogger "github.com/kataras/iris/middleware/logger"
	//_ "github.com/valyala/fasthttp"
)

var (
	reParam = regexp.MustCompile("PARAM_[0-9]*")
)

func startServer() {
	go func() {

		logInfof("Dash server starting on \"%v\"\n", *portFlag)
		folderPath, err := osext.ExecutableFolder()
		if err != nil {
			panic(err)
		}

		app := iris.New()
		r, close := newRequestLogger(folderPath)
		defer close()

		app.Use(r)

		app.RegisterView(iris.HTML(folderPath+"/templates", ".html").Reload(true))

		// Register custom handler for specific http errors.
		app.OnErrorCode(iris.StatusInternalServerError, func(ctx context.Context) {
			// .Values are used to communicate between handlers, middleware.
			errMessage := ctx.Values().GetString("error")
			if errMessage != "" {
				ctx.Writef("Internal server error: %s", errMessage)
				return
			}

			ctx.Writef("(Unexpected) internal server error")
		})
		app.StaticWeb("/js", folderPath+"/static/js")

		app.Get("/*filepath", func(ctx context.Context) {
			filePath := ctx.Params().Get("filepath")
			if filePath == "" {
				ctx.NotFound()
				return
			}
			for k, v := range ctx.URLParams() {
				ctx.ViewData(k, v)
			}
			if err := ctx.View(filePath); err != nil {
				ctx.StatusCode(iris.StatusInternalServerError)
				ctx.WriteString(err.Error())
			}
		})

		app.Get("/data/:name", func(ctx context.Context) {
			//ctx.Render("client.html", clientPage{"Client Page", ctx.HostString()})
			//connString := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%d", *server, *user, *password, *port)
			dataName := ctx.Params().Get("name")
			if dataName == "" {
				ctx.NotFound()
				return
			}
			conn, err := sql.Open("mssql", *dsnFlag)
			if err != nil {
				ctx.Values().Set("error", "Connection error: "+err.Error())
				ctx.StatusCode(iris.StatusInternalServerError)
				return
			}
			defer conn.Close()
			buf, err := ioutil.ReadFile(fmt.Sprintf(folderPath+"/Data/%s.sql", dataName))
			if err != nil {
				ctx.Values().Set("error", "Read file error: "+err.Error())
				ctx.StatusCode(iris.StatusInternalServerError)
				return
			}
			//fmt.Println(string(buf))
			stmt, err := conn.Prepare(string(buf))
			if err != nil {
				ctx.Values().Set("error", "Prepare error: "+err.Error())
				ctx.StatusCode(iris.StatusInternalServerError)
				return
			}
			defer stmt.Close()

			names := make([]string, 0)
			for k := range ctx.URLParams() {
				if reParam.MatchString(strings.ToUpper(k)) {
					names = append(names, k)
				}
			}
			sort.Strings(names)

			params := make([]interface{}, 0)
			for _, v := range names {
				params = append(params, ctx.URLParam(v))
			}

			rows, err := stmt.Query(params...)
			if err != nil {
				ctx.Values().Set("error", "Query failedr: "+err.Error())
				ctx.StatusCode(iris.StatusInternalServerError)
				return
			}
			defer rows.Close()

			columns, err := rows.Columns()
			if err != nil {
				ctx.Values().Set("error", "Query failedr: "+err.Error())
				ctx.StatusCode(iris.StatusInternalServerError)
				return
			}

			count := len(columns)

			tableData := make([]map[string]interface{}, 0)
			values := make([]interface{}, count)
			valuePtrs := make([]interface{}, count)
			for rows.Next() {
				for i := 0; i < count; i++ {
					valuePtrs[i] = &values[i]
				}
				rows.Scan(valuePtrs...)
				entry := make(map[string]interface{})
				for i, col := range columns {
					var v interface{}
					val := values[i]
					b, ok := val.([]byte)
					if ok {
						v = string(b)
					} else {
						v = val
					}
					entry[col] = v
				}
				tableData = append(tableData, entry)
			}
			ctx.Header("Cache-Control", "no-cache")
			ctx.Header("Pragma", "no-cache")
			ctx.Header("Access-Control-Allow-Origin", "*")

			ctx.JSON(tableData)

			if *debugFlag {
				dump, err := httputil.DumpRequest(ctx.Request(), true)
				s := "Request:\n>>>>>>>>>>>>>>>>>\n" + string(dump) + "\n"
				if err != nil {
					s = s + "\n" + "Request Dump Error: " + err.Error() + "\n"
				}
				s = s + "<<<<<<<<<<<<<<<<<\n"
				bTmp, _ := json.Marshal(tableData)
				s = s + "\nResponse:\n>>>>>>>>>>>>>>>>>\n" + string(bTmp) + "\n"
				if err != nil {
					s = s + "\n" + "Response Dump Error: " + err.Error() + "\n"
				}
				s = s + "<<<<<<<<<<<<<<<<<\n"

				filename := fmt.Sprintf("%s\\log\\%s.dump", folderPath, time.Now().Format("2006-01-02T15-04-05-999999999Z07-00"))
				_ = ioutil.WriteFile(filename, []byte(s), 0644)
			}
		})
		certName := *certFlag
		keyName := *keyFlag
		if (certName == "") || (keyName == "") {
			certName = folderPath + "/cert.pem"
			keyName = folderPath + "/key.pem"
		}
		app.Run(iris.TLS(fmt.Sprintf(":%v", *portFlag), certName, keyName), iris.WithoutVersionChecker)
	}()
}
func stopServer() {

}

func todayFilename(filePath string) string {
	today := time.Now().Format("2006_01_02")
	return filePath + "/log/" + today + ".log"
}

func newLogFile(filePath string) *os.File {
	filename := todayFilename(filePath)
	dir, _ := filepath.Split(filename)
	os.MkdirAll(dir, os.ModeDir)
	// open an output file, this will append to the today's file if server restarted.
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	return f
}

func newRequestLogger(filePath string) (h context.Handler, close func() error) {
	close = func() error { return nil }

	c := irisLogger.Config{
		Status:            true,
		IP:                true,
		Method:            true,
		Path:              true,
		Columns:           true,
		MessageContextKey: "error",
	}

	logFile := newLogFile(filePath)
	close = func() error {
		err := logFile.Close()
		return err
	}

	c.LogFunc = func(now time.Time, latency time.Duration, status, ip, method, path string, responseLength int, message interface{}) {
		//		output := irisLogger.Columnize(now.Format("2006/01/02 - 15:04:05"), latency, status, ip, method, path, message)
		//		logFile.Write([]byte(output))
		line := fmt.Sprintf("%s | %v | %4v | %s | %s | %s | %v", now.Format("2006/01/02 - 15:04:05"), latency, status, ip, method, path, responseLength)
		if message != nil {
			line += fmt.Sprintf(" | %v", message)
		}
		line += "\n"

		logFile.Write([]byte(line))
	}

	h = irisLogger.New(c)

	return
}
