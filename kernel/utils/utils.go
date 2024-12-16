package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sisoputnfrba/tp-golang/kernel/globals"
)

/*---------------------- ESTRUCTURAS ----------------------*/
type Interrupcion struct {
	Pid          int    `json:"pid"`
	Tid          int    `json:"tid"`
	Interrupcion string `json:"interrupcion"`
}

type PCB struct {
	Pid   int
	Tid   []int
	Mutex []Mutex
}

type TCB struct {
	Pid             int
	Tid             int
	Prioridad       int
	HilosBloqueados []int
}

type Mutex struct {
	Nombre         string
	Bloqueado      bool
	HiloUsando     int
	colaBloqueados []TCB
}

type CambioHilos struct {
	Pid       int `json:"pid"`
	TidActual int `json:"tidActual"`
	TidCambio int `json:"tidAEjecutar"`
}

type IOsyscall struct {
	TiempoIO int `json:"tiempoIO"`
	Pid      int `json:"pid"`
	Tid      int `json:"tid"`
}

// Request
type KernelRequest struct {
	Size int `json:"size"`
	Pid  int `json:"pid"`
}

type TCBRequestMemory struct {
	Pid  int    `json:"pid"`
	Tid  int    `json:"tid"`
	Path string `json:"path"`
}

type TCBRequest struct {
	Pid int `json:"pid"`
	Tid int `json:"tid"`
}

type PCBRequest struct {
	Pid int `json:"pid"`
}

type MutexRequest struct {
	Pid   int    `json:"pid"`
	Tid   int    `json:"tid"`
	Mutex string `json:"mutex"`
}

// Response
type IniciarProcesoResponse struct {
	Path      string `json:"path"`
	Size      int    `json:"size"`
	Prioridad int    `json:"prioridad"`
	PidActual int    `json:"pidActual"`
	TidActual int    `json:"tidActual"`
}

type CrearHiloResponse struct {
	Pid       int    `json:"pid"`
	Tid       int    `json:"tid"` // del hilo que ejecuto la funcion
	Path      string `json:"path"`
	Prioridad int    `json:"prioridad"`
}

type estadoMemoria struct {
	Estado int `json:"estado"`
}

type Proceso struct{
	PCB PCB
	Size int
	Path string
	Prioridad int
}

/*-------------------- COLAS GLOBALES --------------------*/

var colaProcesosSinIniciar []Proceso

//var colaNewproceso []PCB
var colaProcesosInicializados []PCB
var colaExitproceso []PCB

var colaReadyHilo []TCB
var colaExecHilo []TCB
var colaBlockHilo []TCB
var colaExitHilo []TCB

/*-------------------- MUTEX GLOBALES --------------------*/
var mutexProcesosSinIniciar sync.Mutex
//var mutexColaNewproceso sync.Mutex
var mutexColaExitproceso sync.Mutex
var mutexColaProcesosInicializados sync.Mutex

var mutexColaReadyHilo sync.Mutex
var mutexColaExecHilo sync.Mutex
var mutexColaBlockHilo sync.Mutex
var mutexColaExitHilo sync.Mutex

var mutexEsperarCompactacion sync.Mutex

/*-------------------- VAR GLOBALES --------------------*/

var (
	nextPid = 1
)

var nextTid []int

var ConfigKernel *globals.Config

/*---------------------- CANALES ----------------------*/

//var esperarFinProceso bool = true
var esperarFinCompactacion bool = true

//VER CANAL esperarFinProceso QUE LO USAMOS PARA SABER CUANDO FINALIZA UN PROCESO Y ASI PODER INICIALIZAR OTRO PERO NOS ESTA SIENDO BLOQUEANTE

/*---------------------- FUNCIONES ----------------------*/
//	INICIAR CONFIGURACION Y LOGGERS

func IniciarConfiguracion(filePath string) *globals.Config {
	var config *globals.Config
	configFile, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer configFile.Close()

	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)

	return config
}

func ConfigurarLogger() {
	logFile, err := os.OpenFile("tp.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)
}

// INICIAR MODULO

func init() {

	/*	slog.SetLogLoggerLevel(slog.LevelInfo)
		slog.SetLogLoggerLevel(slog.LevelWarn)
		slog.SetLogLoggerLevel(slog.LevelError)
	SE SETEA EL NIVEL MINIMO DE LOGS A IMPRIMIR POR CONSOLA*/

	ConfigKernel = IniciarConfiguracion(os.Args[1])

	if ConfigKernel != nil {

		switch ConfigKernel.LogLevel {
		case "INFO":
			slog.SetLogLoggerLevel(slog.LevelInfo)
		case "WARN":
			slog.SetLogLoggerLevel(slog.LevelWarn)
		case "ERROR":
			slog.SetLogLoggerLevel(slog.LevelError)
		case "DEBUG":
			slog.SetLogLoggerLevel(slog.LevelDebug)
		default:
			slog.SetLogLoggerLevel(slog.LevelDebug)
		}

		procesoInicial(ConfigKernel.ArchivoInicial, ConfigKernel.SizeInicial)

		if ConfigKernel.AlgoritmoPlanificacion == "FIFO" {
			go ejecutarHilosFIFO()
		} else if ConfigKernel.AlgoritmoPlanificacion == "PRIORIDADES" {
			go ejecutarHilosPrioridades()
		} else if ConfigKernel.AlgoritmoPlanificacion == "CMN" {
			go ejecutarHilosColasMultinivel(ConfigKernel.Quantum)
		} else {
			log.Fatalf("Algoritmo de planificacion no valido")
		}
	} else {
		log.Fatalf("Configuracion no inicializada, segui participando...")
	}
}

/*---------- FUNCIONES PROCESOS ----------*/

func procesoInicial(path string, size int) {

	pcb := createPCB()
	//encolarProcesoNew(pcb)
	var proceso Proceso = Proceso{pcb, size, path, 0}
	mutexProcesosSinIniciar.Lock()
	colaProcesosSinIniciar = append(colaProcesosSinIniciar, proceso)
	mutexProcesosSinIniciar.Unlock()
	inicializarProceso(path, size, 0, pcb)

}

func createPCB() PCB {
	nextPid++

	return PCB{
		Pid:   nextPid - 1,
		Tid:   []int{},
		Mutex: []Mutex{},
	}
}

func getPCB(pid int) (PCB, error) {
    for _, pcb := range colaProcesosInicializados {
        if pcb.Pid == pid {
            return pcb, nil
        }
    }
    return PCB{}, fmt.Errorf("PCB with pid %d not found", pid)
}

func CrearProceso(w http.ResponseWriter, r *http.Request) {

	var proceso IniciarProcesoResponse
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&proceso)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := proceso.Path
	size := proceso.Size
	prioridad := proceso.Prioridad
	pidActual := proceso.PidActual
	tidActual := proceso.TidActual

	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <PROCESS_CREATE> ##", pidActual, tidActual)

	
	iniciarProceso(path, size, prioridad)

	tcbActual := getTCB(pidActual, tidActual)

	enviarTCBCpu(tcbActual)

	w.WriteHeader(http.StatusOK)
}

func iniciarProceso(path string, size int, prioridad int) {

	pcb := createPCB()
	//encolarProcesoNew(pcb)
	var proceso Proceso = Proceso{pcb, size, path, prioridad}
	mutexProcesosSinIniciar.Lock()
	colaProcesosSinIniciar = append(colaProcesosSinIniciar, proceso)
	mutexProcesosSinIniciar.Unlock()
	inicializarProceso(path, size, prioridad, pcb)
	//go inicializarProceso(path, size, prioridad, pcb)

}


func inicializarProceso(path string, size int, prioridad int, pcb PCB){

	log.Printf(" ## (<PID>:%d) Se crea el proceso - Estado: NEW ##", pcb.Pid)

	
	if esElPrimerProcesoSinIniciar(pcb) {
		
		estadoMemoria := consultaEspacioAMemoria(size, pcb)
	
		if estadoMemoria == HayEspacio{
			nextTid = append(nextTid, 0)
			tcb := createTCB(pcb.Pid, prioridad) 
			pcb.Tid = append(pcb.Tid, tcb.Tid)   

			enviarTCBMemoria(tcb, path)

			colaProcesosSinIniciar = colaProcesosSinIniciar[1:]

			//quitarProcesoNew(pcb)
			encolarProcesoInicializado(pcb)
			encolarReady(tcb)

		}else if estadoMemoria == Compactar{

			mutexEsperarCompactacion.Lock()
			esperarFinCompactacion = false
			mutexEsperarCompactacion.Unlock()

			compactar()
			inicializarProceso(path, size, prioridad, pcb)

			mutexEsperarCompactacion.Lock()
			esperarFinCompactacion = true
			mutexEsperarCompactacion.Unlock()

		}else if estadoMemoria == NoHayEspacio{
			log.Printf("## (<PID: %d >) NO HAY PARTICIONES DISPONIBLES PARA SU TAMANIO", pcb.Pid)
		}else{
			slog.Error("Error en el estado de la memoria")
		}
	}
}


const (
	HayEspacio   int = 1
	Compactar    int = 2
	NoHayEspacio int = 3
)

/*func inicializarProceso(path string, size int, prioridad int, pcb PCB) {
	for {
		if esperarFinProceso {
			if esElPrimerProcesoEnNew(pcb) {
				estadoMemoria := consultaEspacioAMemoria(size, pcb)
				if estadoMemoria == HayEspacio {
					nextTid = append(nextTid, 0)
					tcb := createTCB(pcb.Pid, prioridad) // creamos hilo main
					pcb.Tid = append(pcb.Tid, tcb.Tid)   // agregamos el hilo a la listas de hilos del proceso

					enviarTCBMemoria(tcb, path)

					quitarProcesoNew(pcb)
					encolarProcesoInicializado(pcb)
					encolarReady(tcb)
					break
				} else if estadoMemoria == Compactar {
					mutexEsperarCompactacion.Lock()
					esperarFinCompactacion = false
					mutexEsperarCompactacion.Unlock()

					//for {
						//if len(colaExecHilo) == 0 {
							compactar()
						//	break
						//}
					//}

					mutexEsperarCompactacion.Lock()
					esperarFinCompactacion = true
					mutexEsperarCompactacion.Unlock()
					
				} else if estadoMemoria == NoHayEspacio {
					log.Printf("## (<PID: %d >) NO HAY PARTICIONES DISPONIBLES PARA SU TAMANIO", pcb.Pid)
					esperarFinProceso = false
				} else{
					slog.Error("Error en el estado de la memoria")
				}

			}
		}
	}
}*/

/*func esElPrimerProcesoEnNew(pcb PCB) bool {
	return colaNewproceso[0].Pid == pcb.Pid
}*/

func esElPrimerProcesoSinIniciar(pcb PCB) bool {
	return colaProcesosSinIniciar[0].PCB.Pid == pcb.Pid
}

func compactar(){

	puerto := ConfigKernel.PuertoMemoria
	ip := ConfigKernel.IpMemoria

	log.Printf("## Se solicita compactar la memoria ##")

	url := fmt.Sprintf("http://%s:%d/compactacion", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(nil))

	if err != nil {
		slog.Error("error enviando compactar el proceso")
	}

	if resp.StatusCode != http.StatusOK {
		slog.Warn("Error en el proceso de compactar") //Se hace aca, porque si lo ponemos como un else de la consulta a memoria, ante cualquier error responde esto
	}
}

func FinalizarProceso(w http.ResponseWriter, r *http.Request) {
	var hilo TCBRequest
	decoder := json.NewDecoder(r.Body)

	err := decoder.Decode(&hilo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := hilo.Pid
	tid := hilo.Tid
	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <PROCESS_EXIT> ##", pid, tid)

	if tid == 0 {
		err = exitProcess(pid)
	} else {
		slog.Warn("El hilo no es el principal, no se puede ejecutar esta instruccion")
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func exitProcess(pid int) error { //Consulta de nico: teoricamente si encuentra un hilo en block no deberia estar en ninguna otra, no?

	
	for _, tcb := range colaReadyHilo {
		if tcb.Pid == pid {
			exitHilo(pid, tcb.Tid)
		}
	} // LO PUSE ASI PORQUE NO SOLO HABIA QUE MOVER A EXIT SINO TAMBIEN AVISAR QUE FINALIZA (es decir lo que hace la funcion exit proceses)

	for _, tcb := range colaExecHilo {
		if tcb.Pid == pid {
			exitHilo(pid, tcb.Tid)
		}
	}

	for _, tcb := range colaBlockHilo {
		if tcb.Pid == pid {
			exitHilo(pid, tcb.Tid)
		}
	}

	pcb, _ := getPCB(pid)
	quitarProcesoInicializado(pcb)
	encolarProcesoExit(pcb)


	resp := enviarProcesoFinalizadoAMemoria(pcb)

	if resp == nil {
		// Notificar a traves del canal
		//esperarFinProceso = true
		if len(colaProcesosSinIniciar) > 0 {
			proceso := colaProcesosSinIniciar[0]
			inicializarProceso(proceso.Path, proceso.Size, proceso.Prioridad, proceso.PCB)
		}

	} else {
		slog.Error("Error al enviar el proceso finalizado a memoria")
		return fmt.Errorf("error al enviar el proceso finalizado a memoria")
	}

	return nil
}

func enviarProcesoFinalizadoAMemoria(pcb PCB) error {
	memoryRequest := PCBRequest{Pid: pcb.Pid}

	puerto := ConfigKernel.PuertoMemoria
	ip := ConfigKernel.IpMemoria

	body, err := json.Marshal(&memoryRequest)

	if err != nil {
		slog.Error("Error codificando" + err.Error())
		return err
	}

	url := fmt.Sprintf("http://%s:%d/terminateProcess", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))

	if err != nil {
		slog.Error("Error enviando TCB. ip: %s - puerto: %d", ip, puerto)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Error en la respuesta del módulo de CPU - status_code: %d ", resp.StatusCode)
		return err
	}

	return nil
}

func consultaEspacioAMemoria(size int, pcb PCB) int {
	var memoryRequest KernelRequest
	memoryRequest.Size = size
	memoryRequest.Pid = pcb.Pid
	puerto := ConfigKernel.PuertoMemoria
	ip := ConfigKernel.IpMemoria

	body, err := json.Marshal(memoryRequest)

	if err != nil {
		slog.Error("Fallo el proceso: error codificando " + err.Error())

	}

	url := fmt.Sprintf("http://%s:%d/createProcess", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))

	if err != nil {
		slog.Error("error enviando tamanio del proceso", slog.Int("pid", pcb.Pid), slog.String("ip", ip), slog.Int("puerto", puerto))

	}
	var estado estadoMemoria

	err = json.NewDecoder(resp.Body).Decode(&estado)

	if err != nil {
		return -1
	}
	return estado.Estado
}

/*func encolarProcesoNew(pcb PCB) {
	mutexColaNewproceso.Lock()
	colaNewproceso = append(colaNewproceso, pcb)
	mutexColaNewproceso.Unlock()

	log.Printf("## (<PID %d>) Se crea el proceso - Estado: NEW", pcb.Pid)
}*/

func encolarProcesoExit(pcb PCB) {

	mutexColaExitproceso.Lock()
	colaExitproceso = append(colaExitproceso, pcb)
	mutexColaExitproceso.Unlock()

	log.Printf(" ## (<PID: %d>) finaliza el Proceso - Estado: EXIT ##", pcb.Pid)


}

func encolarProcesoInicializado(pcb PCB) {

	mutexColaProcesosInicializados.Lock()
	colaProcesosInicializados = append(colaProcesosInicializados, pcb)
	mutexColaProcesosInicializados.Unlock()

}

func quitarProcesoInicializado(pcb PCB) {
	mutexColaProcesosInicializados.Lock()
	for i, p := range colaProcesosInicializados {
		if p.Pid == pcb.Pid {
			colaProcesosInicializados = append(colaProcesosInicializados[:i], colaProcesosInicializados[i+1:]...)
		}
	}
	mutexColaProcesosInicializados.Unlock()
}

/*func quitarProcesoNew(pcb PCB) {
	mutexColaNewproceso.Lock()
	for i, p := range colaNewproceso {
		if p.Pid == pcb.Pid {
			colaNewproceso = append(colaNewproceso[:i], colaNewproceso[i+1:]...)
		}
	}
	mutexColaNewproceso.Unlock()
}*/

/*---------- FUNCIONES HILOS ----------*/

func createTCB(pid int, prioridad int) TCB {
	nextTid[pid - 1]++

	return TCB{
		Pid:             pid,
		Tid:             nextTid[pid - 1] - 1,
		Prioridad:       prioridad,
		HilosBloqueados: []int{},
	}
}

func getTCB(pid int, tid int) TCB {
	for _, tcb := range colaReadyHilo {
		if tcb.Pid == pid && tcb.Tid == tid {
			return tcb
		}
	}
	for _, tcb := range colaExecHilo {
		if tcb.Pid == pid && tcb.Tid == tid {
			return tcb
		}
	}
	for _, tcb := range colaBlockHilo {
		if tcb.Pid == pid && tcb.Tid == tid {
			return tcb
		}
	}
	return TCB{}
}

func removeTid(tids []int, tid int) []int {
	for i, t := range tids {
		if t == tid {
			return append(tids[:i], tids[i+1:]...)
		}
	}
	return tids
}

func CrearHilo(w http.ResponseWriter, r *http.Request) {
	var hilo CrearHiloResponse
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&hilo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tidActual := hilo.Tid
	pidActual := hilo.Pid
	path := hilo.Path
	prioridad := hilo.Prioridad

	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <THREAD_CREATE> ##", pidActual, tidActual)

	err = iniciarHilo(pidActual, path, prioridad)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tcbActual := getTCB(pidActual, tidActual)
	enviarTCBCpu(tcbActual)

	w.WriteHeader(http.StatusOK)
}

func iniciarHilo(pid int, path string, prioridad int) error {

	tcb := createTCB(pid, prioridad)

	enviarTCBMemoria(tcb, path)

	pcb, _ := getPCB(pid)
	pcb.Tid = append(pcb.Tid, tcb.Tid)
	actualizarPCB(pcb)
	encolarReady(tcb)
	log.Printf("## (<PID %d>:<TID %d>) Se crea el Hilo - Estado: READY", tcb.Pid, tcb.Tid)
	return nil
}

func FinalizarHilo(w http.ResponseWriter, r *http.Request) { //pedir a cpu que nos pase PID Y TID del hilo
	var hilo TCBRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&hilo)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := hilo.Pid
	tid := hilo.Tid

	log.Printf("## (<PID:%d>:<TID:%d>) - Solicito syscall: <THREAD_EXIT> ##", pid, tid)

	err = exitHilo(pid, tid)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func CancelarHilo(w http.ResponseWriter, r *http.Request) {
	var hilosCancel CambioHilos
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&hilosCancel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := hilosCancel.Pid
	tid := hilosCancel.TidActual
	tidEliminar := hilosCancel.TidCambio
	log.Printf("## (<PID:%d>:<TID:%d>) - Solicito syscall: <THREAD_CANCEL> ##", pid, tid)

	err = exitHilo(pid, tidEliminar)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tcbActual := getTCB(pid, tid)

	enviarTCBCpu(tcbActual)

	w.WriteHeader(http.StatusOK)
}

func EntrarHilo(w http.ResponseWriter, r *http.Request) { //debe ser del mismo proceso

	var hilosJoin CambioHilos

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&hilosJoin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := hilosJoin.Pid
	tidActual := hilosJoin.TidActual
	tidAEjecutar := hilosJoin.TidCambio
	tcbAEjecutar := getTCB(pid, tidAEjecutar)
	tcbActual := getTCB(pid, tidActual)
	log.Printf("## (<PID:%d>:<TID:%d>) - Solicito syscall: <THREAD_JOIN> ##", pid, tidActual)

	if(isInReady(tcbAEjecutar)){
	err = joinHilo(pid, tidActual, tidAEjecutar)
	}else{
		log.Printf("## (<PID:%d>:<TID:%d>) - No se puede hacer join con un hilo que no existe ##", pid, tidActual)
		enviarTCBCpu(tcbActual)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

}

func exitHilo(pid int, tid int) error {
	hilo := getTCB(pid, tid)
	pcb, _ := getPCB(pid)
	pcb.Tid = removeTid(pcb.Tid, tid)
	actualizarPCB(pcb)

	switch {
	case isInExec(hilo):
		quitarExec(hilo)
	case isInReady(hilo):
		quitarReady(hilo)
	case isInBlock(hilo):
		quitarBlock(hilo)
	}

	encolarExit(hilo)

	for _, tidBloqueado := range hilo.HilosBloqueados {
		desbloquearHilosJoin(tidBloqueado, pid)
	}
	if tieneMutexAsignado(pcb, hilo) {
		mutexUsando := getMutexUsando(pcb, hilo)
		unlockMutex(pcb, hilo, mutexUsando.Nombre)
	}

	err := enviarHiloFinalizadoAMemoria(hilo)
	if err != nil {
		log.Printf("Error al enviar hilo finalizado a memoria, pid: %d - tid: %d", hilo.Pid, hilo.Tid)
		return err
	}

	return nil
}

func isInExec(hilo TCB) bool {
	_, err := buscarPorPidYTid(colaExecHilo, hilo.Pid, hilo.Tid)
	return err == nil
}

func isInReady(hilo TCB) bool {
	_, err := buscarPorPidYTid(colaReadyHilo, hilo.Pid, hilo.Tid)
	return err == nil
}

func isInBlock(hilo TCB) bool {
	_, err := buscarPorPidYTid(colaBlockHilo, hilo.Pid, hilo.Tid)
	return err == nil
}

func tieneMutexAsignado(pcb PCB, hilo TCB) bool {
	for _, mutex := range pcb.Mutex {
		if mutex.HiloUsando == hilo.Tid {
			return true
		}
	}
	return false
}

func getMutexUsando(pcb PCB, hilo TCB) Mutex {
	for _, mutex := range pcb.Mutex {
		if mutex.HiloUsando == hilo.Tid {
			return mutex
		}
	}
	return Mutex{}
}

func joinHilo(pid int, tidActual int, tidAEjecutar int) error {
	tcbActual := getTCB(pid, tidActual)
	tcbAEjecutar := getTCB(pid, tidAEjecutar)

	tcbAEjecutar.HilosBloqueados = append(tcbAEjecutar.HilosBloqueados, tidActual)
	actualizarTCB(tcbAEjecutar)

	quitarExec(tcbActual)
	encolarBlock(tcbActual, "PTHREAD_JOIN")

	return nil
}

func actualizarTCB(tcb TCB) {
	for i, hilo := range colaReadyHilo {
		if hilo.Pid == tcb.Pid && hilo.Tid == tcb.Tid {
			colaReadyHilo[i] = tcb
		}
	}
}

/*---------- FUNCIONES HILOS ALGORITMOS PLANIFICACION ----------*/
//FIFO
func ejecutarHilosFIFO() {
	var Hilo TCB
	for {
		if len(colaReadyHilo) > 0 && len(colaExecHilo) == 0 && esperarFinCompactacion {
			Hilo = colaReadyHilo[0]
			ejecutarInstruccion(Hilo)
		}
	}
}

func ejecutarInstruccion(Hilo TCB) {
	if esperarFinCompactacion {
		quitarReady(Hilo)
		encolarExec(Hilo)
		enviarTCBCpu(Hilo)
	}
}

// PRIORIDADES
func ejecutarHilosPrioridades() {
	var Hilo TCB
	for {
		if len(colaReadyHilo) > 0 && len(colaExecHilo) == 0 && esperarFinCompactacion{
			HiloAEjecutar := obtenerHiloMayorPrioridad()
			ejecutarInstruccion(HiloAEjecutar)

		} else if len(colaReadyHilo) > 0 && len(colaExecHilo) >= 1 && esperarFinCompactacion{
			
			if Hilo.Prioridad < colaExecHilo[0].Prioridad {
				enviarInterrupcion(colaExecHilo[0].Pid, colaExecHilo[0].Tid, "Prioridades")
			}
		}
	}
}


func obtenerHiloMayorPrioridad() TCB {

	mutexColaReadyHilo.Lock()

	hiloMayorPrioridad := colaReadyHilo[0]
	for _, hilo := range colaReadyHilo {
		if hilo.Prioridad < hiloMayorPrioridad.Prioridad {
			hiloMayorPrioridad = hilo
		}
	}

	mutexColaReadyHilo.Unlock()

	return hiloMayorPrioridad
}

// MULTICOLAS
func ejecutarHilosColasMultinivel(quantum int) {
	var Hilo TCB
	for {
		if len(colaReadyHilo) > 0 && len(colaExecHilo) == 0 && esperarFinCompactacion{
			Hilo = obtenerHiloMayorPrioridad()
			go comenzarQuantum(Hilo, quantum)
			ejecutarInstruccion(Hilo)
		} else if len(colaReadyHilo) > 0 && len(colaExecHilo) >= 1 && esperarFinCompactacion{
			
			if Hilo.Prioridad < colaExecHilo[0].Prioridad {
				enviarInterrupcion(colaExecHilo[0].Pid, colaExecHilo[0].Tid, "Prioridades")
			}
		}
	}
}

func comenzarQuantum(Hilo TCB, quantum int) {

	 time.Sleep(time.Duration(quantum) * time.Millisecond)

	 if len(colaExecHilo) > 0 {
		if Hilo.Tid == colaExecHilo[0].Tid && Hilo.Pid == colaExecHilo[0].Pid {
			enviarInterrupcion(Hilo.Pid, Hilo.Tid, "Quantum")
		}
	 }
			
}

/*---------- FUNCIONES HILOS ENVIO DE TCB ----------*/

func enviarTCBCpu(tcb TCB) error {
	cpuRequest := TCBRequest{}
	cpuRequest.Pid = tcb.Pid
	cpuRequest.Tid = tcb.Tid

	puerto := ConfigKernel.PuertoCpu
	ip := ConfigKernel.IpCpu

	body, err := json.Marshal(&cpuRequest)

	if err != nil {
		slog.Error("error codificando " + err.Error())
		return err
	}

	url := fmt.Sprintf("http://%s:%d/recibirTcb", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))

	if err != nil {
		slog.Error("Error enviando TCB", slog.String("ip", ip), slog.Int("puerto", puerto), slog.Any("error", err))
		return err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("error en la respuesta del módulo de cpu:" + fmt.Sprintf("%v", resp.StatusCode))
		return err
	}
	return nil
}

func enviarTCBMemoria(tcb TCB, path string) error {

	memoryRequest := TCBRequestMemory{}
	memoryRequest.Pid = tcb.Pid
	memoryRequest.Tid = tcb.Tid
	memoryRequest.Path = "pruebas/" + path

	puerto := ConfigKernel.PuertoMemoria
	ip := ConfigKernel.IpMemoria

	body, err := json.Marshal(&memoryRequest)

	if err != nil {
		slog.Error("error codificando " + err.Error())
		return err
	}

	url := fmt.Sprintf("http://%s:%d/createThread", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))

	if err != nil {
		slog.Error("Error enviando TCB", slog.String("ip", ip), slog.Int("puerto", puerto), slog.Any("error", err))
		return err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("Error en la respuesta del modulo de CPU", slog.Int("status_code", resp.StatusCode))
		return err
	}
	return nil
}

func enviarHiloFinalizadoAMemoria(hilo TCB) error {

	memoryRequest := TCBRequest{}
	memoryRequest.Pid = hilo.Pid
	memoryRequest.Tid = hilo.Tid

	puerto := ConfigKernel.PuertoMemoria
	ip := ConfigKernel.IpMemoria

	body, err := json.Marshal(&memoryRequest)

	if err != nil {
		slog.Error("error codificando" + err.Error())
		return err
	}

	url := fmt.Sprintf("http://%s:%d/terminateThread", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))

	if err != nil {
		log.Printf("Error de envio de TCB finalizado ")
		slog.Error("Error enviando TCB para finalizarlo. ip: %d - puerto: %s", ip, puerto)
		return err
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Error en la respuesta del modulo de Memoria. status_code: %d", resp.StatusCode)
		log.Fatalf("Error en la respuesta del modulo de CPU. status_code: %d", resp.StatusCode)
		return err
	}

	return nil
}

/*---------- FUNCIONES DE ESTADOS DE HILOS ----------*/
func desbloquearHilosJoin(tid int, pid int) {
	for _, hilo := range colaBlockHilo {
		if hilo.Tid == tid && hilo.Pid == pid {
			quitarBlock(hilo)

			encolarReady(hilo)

			log.Printf(" ## (<PID: %d>:<TID: %d>) - Pasa de Block a Ready ##", hilo.Pid, hilo.Tid)
		}
	}
}

func encolarReady(tcb TCB) {

	mutexColaReadyHilo.Lock()
	colaReadyHilo = append(colaReadyHilo, tcb)
	mutexColaReadyHilo.Unlock()

	log.Printf("## (<PID %d>:<TID %d>) Se encola el Hilo - Estado: READY", tcb.Pid, tcb.Tid)

	//go verificarReplanificar(tcb)
}


/*func verificarReplanificar(tcb TCB){

	switch ConfigKernel.AlgoritmoPlanificacion {
	case "FIFO":
		if len(colaExecHilo) == 0 && len(colaReadyHilo) > 0 && esperarFinCompactacion{
			Hilo := colaReadyHilo[0]
			ejecutarInstruccion(Hilo)
			break
		}
	case "PRIORIDADES":
		if len(colaExecHilo) == 0 && len(colaReadyHilo) > 0 && esperarFinCompactacion{
			Hilo := obtenerHiloMayorPrioridad()
			ejecutarInstruccion(Hilo)
			break
		} else if len(colaExecHilo) > 0 && tcb.Prioridad < colaExecHilo[0].Prioridad && esperarFinCompactacion{		 
			enviarInterrupcion(colaExecHilo[0].Pid, colaExecHilo[0].Tid, "Prioridades")
			break
		}
	case "CMN":
		if len(colaExecHilo) == 0 && len(colaReadyHilo) > 0 && esperarFinCompactacion{
			log.Printf("## Se replanifica el hilo despues de hacer ready ##")
			Hilo := obtenerHiloMayorPrioridad()
			go comenzarQuantum(Hilo, ConfigKernel.Quantum)
			ejecutarInstruccion(Hilo)
			break
		} else if len(colaExecHilo) > 0 && tcb.Prioridad < colaExecHilo[0].Prioridad && esperarFinCompactacion {
			enviarInterrupcion(colaExecHilo[0].Pid, colaExecHilo[0].Tid, "Prioridades")
			break
		}
	}
}


func replanificar() {
	switch ConfigKernel.AlgoritmoPlanificacion {
	case "FIFO":
		if len(colaReadyHilo) > 0 && len(colaExecHilo) == 0 && esperarFinCompactacion{
			Hilo := colaReadyHilo[0]
			ejecutarInstruccion(Hilo)
			break
		}
	case "PRIORIDADES":
		if len(colaReadyHilo) > 0 && len(colaExecHilo) == 0 && esperarFinCompactacion{
			Hilo := obtenerHiloMayorPrioridad()
			ejecutarInstruccion(Hilo)
			break
		}
	case "CMN":
		if len(colaReadyHilo) > 0 && len(colaExecHilo) == 0 && esperarFinCompactacion{
			log.Printf("## Se replanifica el hilo despues de hacer exit ##")
			Hilo := obtenerHiloMayorPrioridad()
			go comenzarQuantum(Hilo, ConfigKernel.Quantum)
			ejecutarInstruccion(Hilo)
			break
		}
	}
}*/


func encolarExec(tcb TCB) {
	mutexColaExecHilo.Lock()
	colaExecHilo = append(colaExecHilo, tcb)
	mutexColaExecHilo.Unlock()

	log.Printf("## (<PID %d>:<TID %d>) Se ejecuta el Hilo - Estado: EXEC", tcb.Pid, tcb.Tid)
}

func encolarBlock(tcb TCB, motivo string) {
	mutexColaBlockHilo.Lock()
	colaBlockHilo = append(colaBlockHilo, tcb)
	mutexColaBlockHilo.Unlock()

	log.Printf("(<PID: %d >:<TID: %d >) - Bloqueado por: %s", tcb.Pid, tcb.Tid, motivo)
}

func encolarExit(tcb TCB) {
	mutexColaExitHilo.Lock()
	colaExitHilo = append(colaExitHilo, tcb)
	mutexColaExitHilo.Unlock()

	log.Printf(" ## (<PID: %d>:<TID: %d>) finaliza el hilo - Estado: EXIT ##", tcb.Pid, tcb.Tid)
}

func quitarReady(tcb TCB) {
	mutexColaReadyHilo.Lock()
	colaReadyHilo = eliminarHiloCola(colaReadyHilo, tcb)
	mutexColaReadyHilo.Unlock()
}

func quitarExec(tcb TCB) {
	mutexColaExecHilo.Lock()
	colaExecHilo = eliminarHiloCola(colaExecHilo, tcb)
	mutexColaExecHilo.Unlock()

	//go replanificar()
}

func quitarBlock(tcb TCB) {
	mutexColaBlockHilo.Lock()
	colaBlockHilo = eliminarHiloCola(colaBlockHilo, tcb)
	mutexColaBlockHilo.Unlock()
}

func eliminarHiloCola(colaHilo []TCB, tcb TCB) []TCB {
	var nuevaColaHilo []TCB
	for i, t := range colaHilo {
		if t.Pid == tcb.Pid && t.Tid == tcb.Tid {
			nuevaColaHilo = append(colaHilo[:i], colaHilo[i+1:]...)
		}
	}
	return nuevaColaHilo
}

func obtenerHiloDeCola(colaHilo []TCB, criterio func(TCB) bool) (TCB, error) {
	for i := len(colaHilo) - 1; i >= 0; i-- {
		if criterio(colaHilo[i]) { // aplicamos el criterio de búsqueda
			return colaHilo[i], nil
		}
	}
	return TCB{}, fmt.Errorf("no se encontro el hilo buscado")
}

// Uso de la función para búsqueda por pid y tid
func buscarPorPidYTid(colaHilo []TCB, pid, tid int) (TCB, error) {
	return obtenerHiloDeCola(colaHilo, func(hilo TCB) bool { return hilo.Pid == pid && hilo.Tid == tid })
}

/*---------- FUNCION SYSCALL IO Y DUMP MEMORY ----------*/

func ManejarIo(w http.ResponseWriter, r *http.Request) {
	var ioSyscall IOsyscall
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&ioSyscall)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := ioSyscall.Pid
	tid := ioSyscall.Tid
	tiempoIO := ioSyscall.TiempoIO

	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <IO> ##", pid, tid)

	tcb := getTCB(pid, tid)

	quitarExec(tcb)
	encolarBlock(tcb, "IO")

	go func() {
		// Simulate IO operation
		time.Sleep(time.Duration(tiempoIO) * time.Millisecond)
		quitarBlock(tcb)
		encolarReady(tcb)
	}()

	w.WriteHeader(http.StatusOK)
}

func DumpMemory(w http.ResponseWriter, r *http.Request) {
	var hilo TCBRequest
	
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&hilo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tid := hilo.Tid
	pid := hilo.Pid
	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <DUMP_MEMORY> ##", pid, tid)

	tcb := getTCB(pid, tid)
	quitarExec(tcb)
	encolarBlock(tcb, "DUMP_MEMORY")

	espacioParaElArchivo, err := enviarDumpMemoryAMemoria(tcb)

	if err != nil {
		slog.Error("Error al enviar el dump memory a memoria")
	}

	if espacioParaElArchivo {
		quitarBlock(tcb)
		encolarReady(tcb)
	} else {
		exitProcess(pid)
	}

	w.WriteHeader(http.StatusOK)

}

func enviarDumpMemoryAMemoria(tcb TCB) (bool, error) {

	memoryRequest := TCBRequest{}
	memoryRequest.Pid = tcb.Pid
	memoryRequest.Tid = tcb.Tid

	puerto := ConfigKernel.PuertoMemoria
	ip := ConfigKernel.IpMemoria

	body, err := json.Marshal(&memoryRequest)

	if err != nil {
		slog.Error("error codificando" + err.Error())
		return false, err
	}

	url := fmt.Sprintf("http://%s:%d/dumpMemory", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		slog.Error("Error enviando TCB para dump memory. ip: %s - puerto: %d", ip, puerto)
		return false, err
	}
	
	body2, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error al leer la respuesta de Memoria:", err)
		return false, err
	}
	var resultado map[string]bool
	err = json.Unmarshal(body2, &resultado)
	if err != nil {
		fmt.Println("Error al procesar la respuesta JSON:", err)
		return false, err
	}
	respuesta := resultado["resultado"]

	return respuesta , nil
}

/*---------- FUNCIONES SYSCALL MUTEX ----------*/

func CrearMutex(w http.ResponseWriter, r *http.Request) {
	var mutex MutexRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&mutex)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := mutex.Pid
	tid := mutex.Tid
	mutexNombre := mutex.Mutex

	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <MUTEX_CREATE> ##", pid, tid)

	mutexNuevo := mutexCreate(mutexNombre)
	pcb, _ := getPCB(pid)
	pcb.Mutex = append(pcb.Mutex, mutexNuevo)
	actualizarPCB(pcb) //actualizo la PCB con los nuevos mutex
	tcbActual := getTCB(pid, tid)
	enviarTCBCpu(tcbActual)

	w.WriteHeader(http.StatusOK)

}

func actualizarPCB(pcb PCB) {
	for i, p := range colaProcesosInicializados {
		if p.Pid == pcb.Pid {
			mutexColaProcesosInicializados.Lock()
			colaProcesosInicializados[i] = pcb
			mutexColaProcesosInicializados.Unlock()
		}
	}
}

func BloquearMutex(w http.ResponseWriter, r *http.Request) {
	var mutex MutexRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&mutex)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := mutex.Pid
	tid := mutex.Tid
	mutexNombre := mutex.Mutex

	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <MUTEX_LOCK> ##", pid, tid)
	
	proceso, _ := getPCB(pid)
	hiloSolicitante := getTCB(pid, tid)

	err = lockMutex(proceso, hiloSolicitante, mutexNombre)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)

}

func LiberarMutex(w http.ResponseWriter, r *http.Request) {
	var mutex MutexRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&mutex)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pid := mutex.Pid
	tid := mutex.Tid
	mutexNombre := mutex.Mutex

	log.Printf("## (<PID:%d>:<TID:%d>) - Solicitó syscall: <MUTEX_UNLOCK> ##", pid, tid)

	proceso, _ := getPCB(pid)
	hiloSolicitante := getTCB(pid, tid)

	err = unlockMutex(proceso, hiloSolicitante, mutexNombre)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	enviarTCBCpu(hiloSolicitante)

	w.WriteHeader(http.StatusOK)
}

func lockMutex(proceso PCB, hiloSolicitante TCB, mutexNombre string) error {

	for _, mutex := range proceso.Mutex { //recorro todos los mutex que hay en el proceso

		if mutex.Nombre == mutexNombre {

			if !mutex.Bloqueado { // si el mutex no esta bloqueado se lo asigno al hilo que lo pidio

				mutex.Bloqueado = true
				mutex.HiloUsando = hiloSolicitante.Tid

				for i, m := range proceso.Mutex {
					if m.Nombre == mutexNombre {
						proceso.Mutex[i] = mutex
						actualizarPCB(proceso)
						break
					}
				}
				if isInExec(hiloSolicitante) {

					enviarTCBCpu(hiloSolicitante)
				} else {
					break
				}

			} else { // si el mutex esta bloqueado, encolo al hilo en la lista de bloqueados del mutex

				mutex.colaBloqueados = append(mutex.colaBloqueados, hiloSolicitante)
				for i, m := range proceso.Mutex {
					if m.Nombre == mutexNombre {
						proceso.Mutex[i] = mutex
						actualizarPCB(proceso)
					}
				}
				quitarExec(hiloSolicitante)
				encolarBlock(hiloSolicitante, "MUTEX")
				break
			}

		} else {
			slog.Warn("El mutex no existe")
			enviarTCBCpu(hiloSolicitante)
			break
		}
	}
	return nil
}

func unlockMutex(proceso PCB, hiloSolicitante TCB, mutexNombre string) error {

	for _, mutex := range proceso.Mutex {

		if mutex.Nombre == mutexNombre {

			if mutex.HiloUsando == hiloSolicitante.Tid {
				mutex.Bloqueado = false
				mutex.HiloUsando = -1

				if len(mutex.colaBloqueados) > 0 {
					hiloDesbloqueado := mutex.colaBloqueados[0]
					mutex.colaBloqueados = mutex.colaBloqueados[1:]
					
					for i, m := range proceso.Mutex {
						if m.Nombre == mutexNombre {
							proceso.Mutex[i] = mutex
							actualizarPCB(proceso)
							break
						}
					}
					quitarBlock(hiloDesbloqueado)
					encolarReady(hiloDesbloqueado)
					lockMutex(proceso, hiloDesbloqueado, mutexNombre)
					return nil 
				}

				for i, m := range proceso.Mutex {
					if m.Nombre == mutexNombre {
						proceso.Mutex[i] = mutex
						actualizarPCB(proceso)
						break
					}
				}

			} else {
				slog.Warn("El hilo solicitante no tiene asignado al mutex")
				break
			}

		} else {
			slog.Warn("El mutex no existe")
			break
		}
	}
	return nil
}

func mutexCreate(nombreMutex string) Mutex {

	return Mutex{
		Nombre:         nombreMutex,
		Bloqueado:      false,
		HiloUsando:     -1, // -1 Indica que no hay ningun hilo usando el mutex
		colaBloqueados: []TCB{},
	}
}

/*---------- FUNCION ENVIAR INTERRUPCION ----------*/

func enviarInterrupcion(pid int, tid int, motivo string) {

	cpuRequest := Interrupcion{}
	cpuRequest.Pid = pid
	cpuRequest.Tid = tid
	cpuRequest.Interrupcion = motivo

	puerto := ConfigKernel.PuertoCpu
	ip := ConfigKernel.IpCpu

	body, err := json.Marshal(&cpuRequest)

	if err != nil {
		slog.Error("error codificando" + err.Error())
		return
	}

	url := fmt.Sprintf("http://%s:%d/interrupcion", ip, puerto)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))

	if err != nil {
		slog.Error("Error enviando interrupcion", slog.String("ip", ip), slog.Int("puerto", puerto), slog.Any("error", err))
		return
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("error en la respuesta del módulo de cpu:" + fmt.Sprintf("%v", resp.StatusCode))
		return
	}
}

func DevolverPidTid(w http.ResponseWriter, r *http.Request) {

	var tcb Interrupcion
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&tcb)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pid := tcb.Pid
	tid := tcb.Tid
	motivo := tcb.Interrupcion
	tcbActual := getTCB(pid, tid)
	log.Printf("## (<PID:%d>:<TID:%d>) - Desalojado por: %s ##", pid, tid, motivo)
	quitarExec(tcbActual)
	encolarReady(tcbActual)

	w.WriteHeader(http.StatusOK)
}

func SegmentationFault(w http.ResponseWriter, r *http.Request) {

	var tcb TCBRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&tcb)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pid := tcb.Pid
	tid := tcb.Tid
	log.Printf("## (<PID:%d>:<TID:%d>) - <SEGMENTATION_FAULT> ##", pid, tid)

	exitProcess(pid)

	w.WriteHeader(http.StatusOK)
}