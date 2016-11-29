package main


import (
	"net/http"
	"log"
	"html/template"
)


type Credentials struct {
	Data string
}

var templates = template.Must(template.ParseFiles("./templates/main.html"))

func helloView(respWriter http.ResponseWriter, request *http.Request){
	log.Println("helloView.")
	err := templates.ExecuteTemplate(respWriter, "main.html", Credentials{Data: "blam"})
	if err != nil {
		http.Error(respWriter, err.Error(), http.StatusInternalServerError)
		return
	}
}

func main() {

	http.Handle("/vendor/", http.FileServer(http.Dir(".")) )
	http.HandleFunc("/", helloView)
	log.Println("Starting server...")
	http.ListenAndServe(":8080", nil)
	log.Println("Server terminated")
}