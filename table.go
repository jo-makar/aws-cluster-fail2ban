package main

import (
	"fmt"
	"net/http"
)


func WriteTable(w *http.ResponseWriter, table map[string]string) error {
	var err error = nil
	write := func(s string) {
		if err != nil {
			return
		}

		_, err = (*w).Write([]byte(s))
		if err != nil {
			ErrorLog(err.Error())
		}
	}

	write("<html>\n")

	write("<head><style>\n")
	write("table, td { border: 1px solid black; border-collapse: collapse; }\n")
	write("</style></head>\n")

	write("<body><table>\n")

	for name, value := range table {
		write(fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>\n", name, value))
	}

	write("</table></body></html>\n")

	return err
}
