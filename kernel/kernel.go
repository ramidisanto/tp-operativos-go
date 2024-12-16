package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/sisoputnfrba/tp-golang/kernel/globals"
	"github.com/sisoputnfrba/tp-golang/kernel/utils"
)

func main() {
	utils.ConfigurarLogger()
	globals.ClientConfig = utils.IniciarConfiguracion(os.Args[1])

	if globals.ClientConfig == nil {
		log.Fatalf("No se pudo cargar la configuración")
	}

	puerto := globals.ClientConfig.Puerto

	//mux := http.NewServeMux()

	http.HandleFunc("POST /crearProceso", utils.CrearProceso)
	http.HandleFunc("POST /finalizarProceso", utils.FinalizarProceso)

	http.HandleFunc("POST /crearHilo", utils.CrearHilo)
	http.HandleFunc("POST /finalizarHilo", utils.FinalizarHilo)
	http.HandleFunc("POST /cancelarHilo", utils.CancelarHilo)
	http.HandleFunc("POST /unirseAHilo", utils.EntrarHilo)

	http.HandleFunc("POST /crearMutex", utils.CrearMutex)
	http.HandleFunc("POST /bloquearMutex", utils.BloquearMutex)
	http.HandleFunc("POST /liberarMutex", utils.LiberarMutex)

	http.HandleFunc("POST /manejarIo", utils.ManejarIo)

	http.HandleFunc("POST /devolverPidTid", utils.DevolverPidTid)

	http.HandleFunc("POST /dumpMemory", utils.DumpMemory)

	http.HandleFunc("POST /segmentationFault", utils.SegmentationFault)

	//Escuchar (bloqueante)
	http.ListenAndServe(":"+strconv.Itoa(puerto), nil)

}
