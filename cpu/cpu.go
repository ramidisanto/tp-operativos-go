package main

import (
	"log"
	"net/http"
	"strconv"
	"os"

	"github.com/sisoputnfrba/tp-golang/cpu/globals"
	"github.com/sisoputnfrba/tp-golang/cpu/utils"
)

func main() {
	utils.ConfigurarLogger()

	globals.ClientConfig = utils.IniciarConfiguracion(os.Args[1])

	if globals.ClientConfig == nil {
		log.Fatalf("No se pudo cargar la configuración")
	}
	puerto := globals.ClientConfig.Puerto

	mux := http.NewServeMux()

	//mux.HandleFunc("/paquete", utils.RecibirPaquete)
	mux.HandleFunc("/recibirTcb", utils.RecibirPIDyTID)
	//mux.HandleFunc("/interrupcion", utils.Interruption)
	mux.HandleFunc("/receiveDataFromMemory", utils.RecieveDataFromMemory)
	mux.HandleFunc("/interrupcion", utils.RecieveInterruption)
	http.ListenAndServe(":"+strconv.Itoa(puerto), mux)

}
