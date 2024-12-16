package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/sisoputnfrba/tp-golang/cpu/globals"
)

// var globales
var wg sync.WaitGroup
var hiloAnt hiloAnterior
var ConfigsCpu *globals.Config
var mutexInterrupt sync.Mutex
var nuevaInterrupcion Interrupt
var memoryData sync.WaitGroup
var dataFromMemory uint32
var flagSegmentationFault bool
var syscallEnviada bool = false

// DEFINICION DE TIPOS
type hiloAnterior struct {
	Pid int
	Tid int
}

type InstructionReq struct {
	Pid int `json:"pid"`
	Tid int `json:"tid"`
	Pc  int `json:"pc"`
}
type DataRead struct {
	Data []byte `json:"data"`
}

type MutexRequest struct {
	Pid   int    `json:"pid"`
	Tid   int    `json:"tid"`
	Mutex string `json:"mutex"`
}

type InstructionResponse struct {
	Instruction string `json:"instruction"`
}
type Interrupcion struct {
	Pid          int    `json:"pid"`
	Tid          int    `json:"tid"`
	Interrupcion string `json:"interrupcion"`
}

/*
type KernelInterrupcion struct { // ver con KERNEL

		Pid    int    `json:"pid"`
		Tid    int    `json:"tid"`
		Motivo string `json:"motivo"`
	}
*/
type Interrupt struct {
	Pid               int
	Tid               int
	flagInterrucption bool
	motivo            string
}
type MemoryRequest struct {
	PID     int    `json:"pid"`
	TID     int    `json:"tid,omitempty"`
	Address uint32 `json:"address"`        //direccion de memoria a leer
	Size    int    `json:"size,omitempty"` //tamaño de la memoria a leer
	Data    []byte `json:"data,omitempty"` //datos a escribir o leer y los devuelvo
	Port    int    `json:"port,omitempty"` //puerto
}

type PCB struct {
	Pid   int
	Base  uint32
	Limit uint32
}

type TCB struct {
	Pid int
	Tid int
	AX  uint32
	BX  uint32
	CX  uint32
	DX  uint32
	EX  uint32
	FX  uint32
	GX  uint32
	HX  uint32
	PC  uint32
}

type contextoEjecucion struct {
	pcb PCB
	tcb TCB
}
type DecodedInstruction struct {
	instruction FuncInctruction
	parameters  []string
}
type FuncInctruction func(*contextoEjecucion, []string) error

type BodyContexto struct {
	Pcb PCB `json:"pcb"`
	Tcb TCB `json:"tcb"`
}

type ProcessCreateBody struct {
	Path     string `json:"path"`
	Size     string `json:"size"`
	Priority string `json:"prioridad"`
}
type KernelExeReq struct {
	Pid int `json:"pid"` // ver cuales son los keys usados en Kernel
	Tid int `json:"tid"`
}
type IOReq struct {
	Tiempo int `json:"tiempoIO"`
	Pid    int `json:"pid"`
	Tid    int `json:"tid"`
}

type IniciarProcesoBody struct {
	Path      string `json:"path"`
	Size      int    `json:"size"`
	Prioridad int    `json:"prioridad"`
	PidActual int    `json:"pidActual"`
	TidActual int    `json:"tidActual"`
}

type CrearHiloBody struct {
	Pid       int    `json:"pid"`
	Path      string `json:"path"`
	Prioridad int    `json:"prioridad"`
}
type EfectoHiloBody struct {
	Pid       int `json:"pid"`
	TidActual int `json:"tidActual"`
	TidCambio int `json:"tidAEjecutar"`
}

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

func init() {
	ConfigsCpu = IniciarConfiguracion(os.Args[1])
	hiloAnt.Pid = -1
	hiloAnt.Tid = -1

}

func ConfigurarLogger() {
	logFile, err := os.OpenFile("tp.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(mw)
}

// FUNCIONES PRINCIPALES
func RecibirPIDyTID(w http.ResponseWriter, r *http.Request) {
	log.Printf("ME LLEGA NUEVO PID Y TID")
	var processAndThreadIDs KernelExeReq
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&processAndThreadIDs)

	if err != nil {
		log.Printf("Error al decodificar el pedido del Kernel: %s\n", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error al decodificar mensaje"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
	wg.Add(1)
	contextoActual := GetContextoEjecucion(processAndThreadIDs.Pid, processAndThreadIDs.Tid)

	InstructionCycle(&contextoActual)

}
func GetContextoEjecucion(pid int, tid int) (context contextoEjecucion) {
	log.Printf("Busca el contexto de ejecucion")

	var contextoDeEjecucion contextoEjecucion
	var reqContext KernelExeReq
	reqContext.Pid = pid
	reqContext.Tid = tid
	reqContextBody, err := json.Marshal(reqContext)

	if err != nil {
		log.Printf("Error al codificar el mensaje de solicitud de contexto de ejecucion")
		return
	}

	log.Printf("PCB : %d TID : %d - Solicita Contexto de Ejecucion", pid, tid)

	url := fmt.Sprintf("http://%s:%d/obtenerContextoDeEjecucion", ConfigsCpu.IpMemoria, ConfigsCpu.PuertoMemoria)

	// Analizar la URL base

	response, err := http.Post(url, "application/json", bytes.NewBuffer(reqContextBody))
	if err != nil {
		log.Printf("error al enviar la solicitud al módulo de memoria: %v", err)
		return
	}
	//defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("error en la respuesta del módulo de memoria: %v", response.StatusCode)
		log.Println(err)
		return
	}

	var contexto BodyContexto
	errorDecode := json.NewDecoder(response.Body).Decode(&contexto)
	if errorDecode != nil {
		log.Println("Error al decodificar el contexto de ejecucion", errorDecode)
		return
	}
	log.Printf("PCB : %d TID : %d - Solicitud Contexto de Ejecucion Exitosa", contexto.Pcb.Pid, contexto.Tcb.Tid)
	contextoDeEjecucion.pcb = contexto.Pcb
	contextoDeEjecucion.tcb = contexto.Tcb
	return contextoDeEjecucion
}

func InstructionCycle(contexto *contextoEjecucion) {
	if hiloAnt.Pid == contexto.pcb.Pid && hiloAnt.Tid == contexto.tcb.Tid {
		log.Printf("Entra a evaluar HILO ANTERIOR")
		if CheckInterrupt(*contexto) {

			if err := RealizarInterrupcion(contexto); err != nil {
				log.Printf("Error al ejecutar la interrupción: %v", err)
			}
			log.Printf("FINALIZA EJECUCION POR INTERRUPCION DE PID : %d y TID : %d ", contexto.pcb.Pid, contexto.tcb.Tid)
			wg.Done()
			return

		}
	}
	wg.Done()
	guardarPidyTid(contexto.pcb.Pid, contexto.tcb.Tid)

	for {
		log.Printf("Instrucción solicitada de PID: %d, TID: %d, PC: %d", contexto.pcb.Pid, contexto.tcb.Tid, contexto.tcb.PC)

		// Fetch
		instructionLine, err := Fetch(contexto.pcb.Pid, contexto.tcb.Tid, &contexto.tcb.PC)
		if err != nil {
			log.Printf("Error al buscar instrucción en PC %d: %v", contexto.tcb.PC, err)
			break
		}

		// Decode
		instruction, err := Decode(instructionLine)
		if err != nil {
			log.Printf("Error en etapa Decode: %v", err)
			break
		}

		log.Printf("Instrucción decodificada: INSTUCCIÓN = %s, PARÁMETROS = %v", instructionLine[0], instruction.parameters)

		// Execute
		log.Printf("## TID: %d - Ejecutando: %s - Parámetros: %v", contexto.tcb.Tid, instructionLine[0], instruction.parameters)
		if err := Execute(contexto, instruction); err != nil {
			log.Printf("Error al ejecutar %v: %v", instructionLine, err)
		}

		

		if flagSegmentationFault {
			flagSegmentationFault = false
			err := EnviarSegmentationFault(contexto.pcb.Pid, contexto.tcb.Tid)
			if err != nil {
				log.Printf("Error al enviar segmentation fault: %v", err)
			}
			break
		}

		if syscallEnviada {
			syscallEnviada = false
			break
		}

		// Check Interrupt
		if CheckInterrupt(*contexto) {
			if err := RealizarInterrupcion(contexto); err != nil {
				log.Printf("Error al ejecutar la interrupcion: %v", err)
			}
			log.Printf("FINALIZA EJECUCION POR INTERRUPCION DE PID : %d y TID : %d ", contexto.pcb.Pid, contexto.tcb.Tid)
			break
		}
	}
}

func EnviarSegmentationFault(pid int, tid int) error {
	kernelReq := KernelExeReq{
		Pid: pid,
		Tid: tid,
	}
	body, err := json.Marshal(kernelReq)
	if err != nil {
		return err
	}
	err2 := EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "segmentationFault")
	if err2 != nil {
		return err2
	}
	return nil
}

func guardarPidyTid(pid int, tid int) {
	hiloAnt.Pid = pid
	hiloAnt.Tid = tid
}

func EnviarPidTidPorInterrupcion(pidActual int, tidActual int, motivo string) error {
	kernelReq := Interrupcion{
		Pid:          pidActual,
		Tid:          tidActual,
		Interrupcion: motivo,
	}
	body, err := json.Marshal(kernelReq)
	if err != nil {
		return err
	}
	err2 := EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "devolverPidTid")
	if err2 != nil {
		return err2
	}
	return nil

}

func RealizarInterrupcion(contexto *contextoEjecucion) error {
	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Panicf("Error al actualizar contexto de ejecucion para la interrupcion")
		return err
	}
	err2 := EnviarPidTidPorInterrupcion(contexto.pcb.Pid, contexto.tcb.Tid, nuevaInterrupcion.motivo)

	if err2 != nil {
		return err2
	}
	log.Printf("## TID: <%d> - Actualizo Contexto Ejecución", contexto.tcb.Tid)
	return nil

}

func Fetch(pid int, tid int, PC *uint32) ([]string, error) {

	reqInstruccion := InstructionReq{
		Pid: pid,
		Tid: tid,
		Pc:  int(*PC),
	}

	reqInstruccionBody, err := json.Marshal(reqInstruccion)
	if err != nil {
		log.Printf("Error al codificar el mensaje de solicitud de instruccion")
		return nil, err
	}

	url := fmt.Sprintf("http://%s:%d/obtenerInstruccion", ConfigsCpu.IpMemoria, ConfigsCpu.PuertoMemoria)
	response, err := http.Post(url, "application/json", bytes.NewBuffer(reqInstruccionBody))
	if err != nil {
		log.Fatalf("error al enviar la solicitud al módulo de memoria: %v", err)
		return nil, err
	}
	defer response.Body.Close()

	// Verificar el estado de la respuesta
	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("error en la respuesta del módulo de memoria: %v", response.StatusCode)
		log.Println(err)
		return nil, err
	}

	// Decodificar la respuesta
	var instructionResponse InstructionResponse
	if err := json.NewDecoder(response.Body).Decode(&instructionResponse); err != nil {
		log.Println("Error al decodificar la instrucción:", err)
		return nil, err
	}

	instructions := strings.Fields(instructionResponse.Instruction) // Separar la instrucción en partes

	log.Printf("PID: %d TID: %d - FETCH - Program Counter: %d", pid, tid, reqInstruccion.Pc)
	*PC++
	return instructions, nil

}

func Decode(instructionLine []string) (DecodedInstruction, error) {
	var SetInstructions map[string]FuncInctruction = map[string]FuncInctruction{
		"SET":            Set,
		"SUM":            Sumar,
		"SUB":            Restar,
		"JNZ":            JNZ,
		"LOG":            Log,
		"DUMP_MEMORY":    DumpMemory,
		"IO":             IO,
		"PROCESS_CREATE": CreateProcess,
		"THREAD_CREATE":  CreateThead,
		"THREAD_JOIN":    JoinThead,
		"THREAD_CANCEL":  CancelThead,
		"THREAD_EXIT":    ThreadExit,
		"PROCESS_EXIT":   ProcessExit,
		"READ_MEM":       Read_Memory,
		"WRITE_MEM":      Write_Memory,
		"MUTEX_CREATE":   MutexCreate,
		"MUTEX_LOCK":     MutexLOCK,
		"MUTEX_UNLOCK":   MutexUNLOCK,
	}

	var instructionDecoded DecodedInstruction

	if len(instructionLine) == 0 {
		return instructionDecoded, fmt.Errorf("instrucción nula")
	}

	functionInstruction, exists := SetInstructions[instructionLine[0]]
	if !exists {
		return instructionDecoded, fmt.Errorf("la instrucción '%s' no existe", instructionLine[0])
	}

	instructionDecoded.instruction = functionInstruction
	instructionDecoded.parameters = instructionLine[1:]

	return instructionDecoded, nil
}

func Execute(ContextoDeEjecucion *contextoEjecucion, intruction DecodedInstruction) error {

	//var syscall bool
	instructionFunc := intruction.instruction
	var parameters []string = intruction.parameters

	err := instructionFunc(ContextoDeEjecucion, parameters)

	if err != nil {
		log.Printf("Error al ejecutar la instruccion : %v", err)
		return err
	}
	return nil

}

func CheckInterrupt(contexto contextoEjecucion) bool {
	mutexInterrupt.Lock()
	if nuevaInterrupcion.flagInterrucption && contexto.pcb.Pid == nuevaInterrupcion.Pid && contexto.tcb.Tid == nuevaInterrupcion.Tid {
		nuevaInterrupcion.flagInterrucption = false
		mutexInterrupt.Unlock()
		return true
	}
	mutexInterrupt.Unlock()
	return false

}

// -------------------------------FUNCIONES DEL SET DE INSTRUCIONES------------------------------------------------
func Set(registrosCPU *contextoEjecucion, parameters []string) error {
	valor := parameters[1]
	registro := parameters[0]

	registers := reflect.ValueOf(&registrosCPU.tcb)

	valorUint, err := strconv.ParseUint(valor, 10, 32)
	if err != nil {
		return err
	}

	err = ModificarValorCampo(registers, registro, uint32(valorUint))
	if err != nil {
		return err
	}

	return nil
}

func Read_Memory(context *contextoEjecucion, parameters []string) error {
	// Obtiene la dirección del registro destino.
	registerDirection := parameters[1]
	registers := reflect.ValueOf(&context.tcb)

	// Obtiene la dirección lógica del registro.
	logicalAddress, err := ObtenerValorCampo(registers, registerDirection)
	if err != nil {
		return err
	}

	// Traduce la dirección lógica a física.
	physicalAddress, err := TranslateAdress(logicalAddress, context.pcb.Base, context.pcb.Limit)
	if err != nil {
		return err
	}

	// Crea la solicitud de lectura de memoria.
	memReq := MemoryRequest{
		Address: physicalAddress,
		PID:     context.pcb.Pid,
		TID:     context.tcb.Tid,
	}

	body, err := json.Marshal(memReq)
	if err != nil {
		log.Printf("Error al codificar el mensaje de solicitud de instrucción: %v", err)
		return err
	}
	log.Printf("## TID: <%d> - Accion: <LEER> - Direccion Fisica: <%d>", context.tcb.Tid, physicalAddress)
	// Envía la solicitud al módulo de memoria.
	url := fmt.Sprintf("http://%s:%d/readMemory", ConfigsCpu.IpMemoria, ConfigsCpu.PuertoMemoria)
	response, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatalf("Error al enviar la solicitud al módulo de memoria: %v", err)
		return err
	}
	defer response.Body.Close()

	// Verifica el código de estado de la respuesta.
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("error en la respuesta del módulo de memoria: %v", response.StatusCode)
	}

	// Decodifica la respuesta y actualiza el registro.
	var dataResponse DataRead
	if err := json.NewDecoder(response.Body).Decode(&dataResponse); err != nil {
		log.Println("Error al decodificar la instrucción:", err)
		return err
	}

	dataToWrite := BytesToUint32(dataResponse.Data)

	if err := ModificarValorCampo(registers, parameters[0], dataToWrite); err != nil {
		return err
	}

	return nil

}

func BytesToUint32(val []byte) uint32 {

	r := uint32(0)
	for i := uint32(0); i < 4; i++ {
		r |= uint32(val[i]) << (8 * i)
	}
	return r

}

func RecieveDataFromMemory(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)

	var data uint32 // ver si esta bien con el tipo que envia memoria

	err := decoder.Decode(&data)
	if err != nil {
		log.Printf("Error al decodificar el pedido de la memorua: %s\n", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error al decodificar mensaje"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))

	dataFromMemory = data
	memoryData.Done()
}

func Write_Memory(context *contextoEjecucion, parameters []string) error {
	// Obtiene el dato del registro.
	dataRegister := parameters[1]
	registers := reflect.ValueOf(&context.tcb)
	data, err := ObtenerValorCampo(registers, dataRegister)
	if err != nil {
		return err
	}

	// Obtiene la dirección del registro.
	addressRegister := parameters[0]
	logicalAddress, err := ObtenerValorCampo(registers, addressRegister)
	if err != nil {
		return err
	}

	// Traduce la dirección lógica a física.
	physicalAddress, err := TranslateAdress(logicalAddress, context.pcb.Base, context.pcb.Limit)
	if err != nil {
		return err
	}

	// Prepara la solicitud de escritura en memoria.
	memReq := MemoryRequest{
		Address: physicalAddress,
		Data:    PasarDeUintAByte(data),
		PID:     context.pcb.Pid,
		TID:     context.tcb.Tid,
	}

	// Codifica la solicitud en formato JSON.
	body, err := json.Marshal(memReq)
	if err != nil {
		return err
	}

	// Envía la solicitud al módulo de memoria.
	log.Printf("## TID: <%d> - Accion: <ESCRIBIR> - Direccion Fisica: <%d>", context.tcb.Tid, physicalAddress)
	if err := EnviarAModulo(ConfigsCpu.IpMemoria, ConfigsCpu.PuertoMemoria, bytes.NewBuffer(body), "writeMemory"); err != nil {
		return err
	}

	return nil
}

func PasarDeUintAByte(val uint32) []byte {
	r := make([]byte, 4)
	for i := uint32(0); i < 4; i++ {
		r[i] = byte((val >> (8 * i)) & 0xff)
	}
	return r
}
func TranslateAdress(direccionLogica uint32, base uint32, limite uint32) (uint32, error) {
	direccionFisica := direccionLogica + base

	if direccionFisica > limite {
		err := fmt.Errorf("Segmentation Fault")
		flagSegmentationFault = true
		return 0, err
	}

	return direccionFisica, nil
}

func Sumar(registrosCPU *contextoEjecucion, parameters []string) error {
	registroDestino := parameters[0]
	registroOrigen := parameters[1]

	registers := reflect.ValueOf(&registrosCPU.tcb)

	originRegister, finalRegister, err := obtenerOperandos(registers, registroDestino, registroOrigen)
	if err != nil {
		return err
	}

	suma := finalRegister + originRegister

	err = ModificarValorCampo(registers, registroDestino, suma)
	if err != nil {
		return err
	}
	return nil
}

func Restar(registrosCPU *contextoEjecucion, parameters []string) error {
	registroDestino := parameters[0]
	registroOrigen := parameters[1]

	registers := reflect.ValueOf(&registrosCPU.tcb)

	originRegister, finalRegister, err := obtenerOperandos(registers, registroDestino, registroOrigen)
	if err != nil {
		return err
	}

	resta := finalRegister - originRegister

	err = ModificarValorCampo(registers, registroDestino, resta)
	if err != nil {
		return err
	}
	return nil
}

func obtenerOperandos(registers reflect.Value, registroDestino string, registroOrigen string) (uint32, uint32, error) {
	valorOrigen, errOrigen := ObtenerValorCampo(registers, registroOrigen)
	if errOrigen != nil {
		return 0, 0, errOrigen
	}

	valorDestino, errDestino := ObtenerValorCampo(registers, registroDestino)
	if errDestino != nil {
		return 0, 0, errDestino
	}

	return valorOrigen, valorDestino, nil
}

func JNZ(registrosCPU *contextoEjecucion, parameters []string) error {
	instruccion := parameters[1]
	registro := parameters[0]

	registers := reflect.ValueOf(&registrosCPU.tcb)

	register, err := ObtenerValorCampo(registers, registro)
	if err != nil {
		return err
	}
	instruction, err := strconv.Atoi(instruccion)
	if err != nil {
		return err
	}

	if register != 0 {
		ModificarValorCampo(registers, "PC", uint32(instruction))
	}

	return nil
}

func Log(registrosCPU *contextoEjecucion, parameters []string) error {
	registro := parameters[0]

	registers := reflect.ValueOf(&registrosCPU.tcb)

	register, err := ObtenerValorCampo(registers, registro)
	if err != nil {
		return err
	}
	log.Printf("El valor del registro %s es %d", registro, register)

	return nil
}

func DumpMemory(contexto *contextoEjecucion, parameters []string) error {
	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Printf("Error al actualizar el contexto de ejecución: %v", err)
		return err
	}

	body, err := json.Marshal(KernelExeReq{
		Pid: contexto.pcb.Pid,
		Tid: contexto.tcb.Tid,
	})
	if err != nil {
		log.Printf("Error al codificar el mensaje: %v", err)
		return err
	}

	if err := EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "dumpMemory"); err != nil {
		return err
	}
	syscallEnviada = true
	return nil
}

func IO(contexto *contextoEjecucion, parameters []string) error {
	tiempo := parameters[0]

	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Printf("Error al actualziar contexto de ejecucion")
		return err
	}

	tiempoReq, err := strconv.Atoi(tiempo)
	if err != nil {
		return err
	}

	body, err := json.Marshal(IOReq{
		Tiempo: tiempoReq,
		Pid:    contexto.pcb.Pid,
		Tid:    contexto.tcb.Tid,
	})
	if err != nil {
		log.Printf("Error al codificar el mernsaje")
		return err
	}

	err = EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "manejarIo")
	if err != nil {
		return err
	}
	syscallEnviada = true
	return nil
}

func CreateProcess(contexto *contextoEjecucion, parameters []string) error {
	archivoInstruct := parameters[0]
	tamArch := parameters[1]
	prioridadTID := parameters[2]

	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Printf("Error al actualizar contexto de ejecución: %v", err)
		return err
	}

	tamArchReal, err := strconv.Atoi(tamArch)
	if err != nil {
		return fmt.Errorf("error al convertir tamaño de archivo: %v", err)
	}

	priorityReal, err := strconv.Atoi(prioridadTID)
	if err != nil {
		return fmt.Errorf("error al convertir prioridad TID: %v", err)
	}

	body, err := json.Marshal(IniciarProcesoBody{
		Path:      archivoInstruct,
		Size:      tamArchReal,
		Prioridad: priorityReal,
		PidActual: contexto.pcb.Pid,
		TidActual: contexto.tcb.Tid,
	})
	if err != nil {
		log.Printf("Error al codificar estructura de creación de proceso: %v", err)
		return err
	}

	if err := EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "crearProceso"); err != nil {
		log.Printf("Error en syscall crearProceso: %v", err)
		return err
	}
	syscallEnviada = true
	return nil
}

func CreateThead(contexto *contextoEjecucion, parameters []string) error { //ESTE ESTA FUNCIONANDO MAL
	archivoInstruct := parameters[0]
	prioridadTID := parameters[1]

	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Printf("Error al actualizar contexto de ejecución: %v", err)
		return err
	}

	priorityReal, err := strconv.Atoi(prioridadTID)
	if err != nil {
		return err
	}

	body, err := json.Marshal(CrearHiloBody{
		Path:      archivoInstruct,
		Pid:       contexto.pcb.Pid,
		Prioridad: priorityReal,
	})
	if err != nil {
		log.Printf("Error al codificar estructura de creación de hilo: %v", err)
		return err
	}

	if err := EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "crearHilo"); err != nil {
		log.Printf("Error syscall THREAD_CREATE: %v", err)
		return err
	}
	syscallEnviada = true
	return nil
}

func JoinThead(contexto *contextoEjecucion, parameters []string) error {
	tid := parameters[0]

	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Printf("Error al actualziar contexto de ejecucion")
		return err
	}

	tidParse, err := strconv.Atoi(tid)
	if err != nil {
		return err
	}

	body, err := json.Marshal(EfectoHiloBody{
		Pid:       contexto.pcb.Pid,
		TidActual: contexto.tcb.Tid,
		TidCambio: tidParse,
	})
	if err != nil {
		log.Printf("Error al codificar estructura de cambio de hilo")
		return err
	}

	err = EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "unirseAHilo")
	if err != nil {
		log.Printf("Error syscall THREAD_JOIN : %v", err)
		return err
	}
	syscallEnviada = true
	return nil
}

func CancelThead(contexto *contextoEjecucion, parameters []string) error {
	tid := parameters[0]

	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Printf("Error al actualziar contexto de ejecucion")
		return err
	}

	tidParse, err := strconv.Atoi(tid)
	if err != nil {
		return err
	}

	body, err := json.Marshal(EfectoHiloBody{
		Pid:       contexto.pcb.Pid,
		TidActual: contexto.tcb.Tid,
		TidCambio: tidParse,
	})
	if err != nil {
		log.Printf("Error al codificar estructura de cancelacion de hilo")
		return err
	}

	err = EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "cancelarHilo")
	if err != nil {
		log.Printf("Error syscall THREAD_CANCEL : %v", err)
		return err
	}
	syscallEnviada = true
	return nil
}

func MutexCreate(contexto *contextoEjecucion, parameters []string) error {
	err := MutexFunction(contexto, parameters, "crearMutex")
	if err != nil {
		return err
	}
	syscallEnviada = true
	return nil
}
func MutexLOCK(contexto *contextoEjecucion, parameters []string) error {
	err := MutexFunction(contexto, parameters, "bloquearMutex")
	if err != nil {
		return err
	}
	syscallEnviada = true
	return nil
}
func MutexUNLOCK(contexto *contextoEjecucion, parameters []string) error {
	err := MutexFunction(contexto, parameters, "liberarMutex")
	if err != nil {
		return err
	}
	syscallEnviada = true
	return nil
}
func MutexFunction(contexto *contextoEjecucion, parameters []string, endpoint string) error {
	recurso := parameters[0]

	err := ActualizarContextoDeEjecucion(contexto)
	if err != nil {
		log.Printf("Error al actualziar contexto de ejecucion")
		return err
	}

	body, err := json.Marshal(MutexRequest{
		Pid:   contexto.pcb.Pid,
		Tid:   contexto.tcb.Tid,
		Mutex: recurso,
	})

	if err != nil {
		log.Printf("Error al codificar estructura de cancelacion de hilo")
		return err
	}
	err = EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), endpoint)
	if err != nil {
		log.Printf("Error syscall THREAD_CANCEL : %v", err)
		return err
	}
	return nil
}

func ThreadExit(contexto *contextoEjecucion, parameters []string) error {
	if err := ActualizarContextoDeEjecucion(contexto); err != nil {
		log.Printf("Error al actualizar contexto de ejecución: %v", err)
		return err
	}

	process := KernelExeReq{
		Pid: contexto.pcb.Pid,
		Tid: contexto.tcb.Tid,
	}

	body, err := json.Marshal(process)
	if err != nil {
		return err
	}

	if err := EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "finalizarHilo"); err != nil {
		return err
	}
	syscallEnviada = true
	return nil
}

func ProcessExit(contexto *contextoEjecucion, parameters []string) error {
	log.Printf("Finalizando proceso PID: %d", contexto.pcb.Pid)

	if err := ActualizarContextoDeEjecucion(contexto); err != nil {
		log.Printf("Error al actualizar contexto de ejecución: %v", err)
		return err
	}

	process := KernelExeReq{
		Pid: contexto.pcb.Pid,
		Tid: contexto.tcb.Tid,
	}

	body, err := json.Marshal(process)
	if err != nil {
		return err
	}

	if err := EnviarAModulo(ConfigsCpu.IpKernel, ConfigsCpu.PuertoKernel, bytes.NewBuffer(body), "finalizarProceso"); err != nil {
		return err
	}

	syscallEnviada = true
	return nil
}

func ActualizarContextoDeEjecucion(contexto *contextoEjecucion) error {
	contextoDeEjecucion := BodyContexto{
		Pcb: contexto.pcb,
		Tcb: contexto.tcb,
	}

	body, err := json.Marshal(contextoDeEjecucion)
	if err != nil {
		log.Printf("Error al codificar el contexto: %v", err)
		return err
	}

	if err := EnviarAModulo(ConfigsCpu.IpMemoria, ConfigsCpu.PuertoMemoria, bytes.NewBuffer(body), "actualizarContextoDeEjecucion"); err != nil {
		return err
	}

	return nil
}

func EnviarAModulo(ipModulo string, puertoModulo int, body io.Reader, endPoint string) error {
	url := fmt.Sprintf("http://%s:%d/%s", ipModulo, puertoModulo, endPoint)
	resp, err := http.Post(url, "application/json", body)

	if err != nil {
		log.Printf("Error enviando mensaje al End point %s - IP:%s - Puerto:%d: %v", endPoint, ipModulo, puertoModulo, err)
		return err
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Error en respuesta del End point %s - IP:%s - Puerto:%d: %s", endPoint, ipModulo, puertoModulo, resp.Status)
		return fmt.Errorf("respuesta inesperada: %s", resp.Status)
	}

	return nil
}

func ObtenerValorCampo(estructura reflect.Value, nombreCampo string) (uint32, error) {
	campoRef := estructura.Elem().FieldByName(nombreCampo)
	if !campoRef.IsValid() {
		err := fmt.Errorf("No se encuentra el campo %s en la estructura", nombreCampo)
		return 0, err
	}
	//estamos suponiendo que jamas se podrá tener un numero de otro tipo que no sea unit32
	return (uint32(campoRef.Uint())), nil

}

func ModificarValorCampo(estructura reflect.Value, nombreCampo string, nuevoValor uint32) error {
	// Solo se aceptan valores de tipo uint32
	campoRef := estructura.Elem().FieldByName(nombreCampo)

	if !campoRef.IsValid() {
		return fmt.Errorf("Campo %s no encontrado en la estructura", nombreCampo)
	}

	if !campoRef.CanSet() {
		return fmt.Errorf("No se puede establecer el valor del campo %s", nombreCampo)
	}

	campoRef.SetUint(uint64(nuevoValor))
	return nil
}

func RecieveInterruption(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var interrupction Interrupcion // Verificar si el tipo es correcto
	if err := decoder.Decode(&interrupction); err != nil {
		log.Printf("Error al decodificar el pedido del Kernel: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Error al decodificar mensaje"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
	log.Printf("## Llega interrupcion al puerto Interrupt")
	wg.Wait()
	log.Printf("## Se registra la interrupcion")
	mutexInterrupt.Lock()
	nuevaInterrupcion.flagInterrucption = true
	nuevaInterrupcion.Pid = interrupction.Pid
	nuevaInterrupcion.Tid = interrupction.Tid
	nuevaInterrupcion.motivo = interrupction.Interrupcion
	mutexInterrupt.Unlock()

}
