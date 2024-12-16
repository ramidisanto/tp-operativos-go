package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/sisoputnfrba/tp-golang/filesystem/globals"
)

/*---------------------- ESTRUCTURAS ----------------------*/
type FSmemoriaREQ struct {
	Data          []byte `json:"data"`
	Tamanio       uint32 `json:"tamanio"`
	NombreArchivo string `json:"nombreArchivo"`
}

type Bitmap struct {
	bits []int
}

type Metadata struct {
	IndexBlock int    `json:"index_block"`
	Size       uint32 `json:"size"`
}

var bitmapGlobal *Bitmap

/*-------------------- VAR GLOBALES --------------------*/
var ConfigFS *globals.Config
var bitmapFilePath string
var bloquesFilePath string

/*---------------------- FUNCIONES CONFIGURACION Y LOGGERS ----------------------*/

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

/*---------------------- FUNCION INIT ----------------------*/

func init() {
	ConfigFS = IniciarConfiguracion(os.Args[1])
	//Al iniciar el modulo se debera validar que existan los archivos bitmap.dat y bloques.dat. En caso que no existan se deberan crear. Caso contrario se deberan tomar los ya existentes.
	if ConfigFS != nil {
		pathFS := ConfigFS.Mount_dir
		blocksSize := ConfigFS.Block_size
		blocksCount := ConfigFS.Block_count
		sizeBloques := (blocksCount * blocksSize) / 8
		sizeBitMap := blocksCount / 8
		asegurarExistenciaDeArchivos(pathFS, sizeBloques, sizeBitMap)
	}

}

/*---------------------- FUNCIONES DE ARCHIVOS ----------------------*/
func asegurarExistenciaDeArchivos(pathFS string, sizeBloques int, sizeBitMap int) {
	//bitmap.dat
	bitmapFilePath = pathFS + "/bitmap.dat"
	if _, err := os.Stat(bitmapFilePath); os.IsNotExist(err) {
		CrearBitmap(pathFS, sizeBitMap)
	} else {
		err := cargarBitmap()
		if err != nil {
			log.Fatalf("Error al cargar el archivo de bitmap '%s': %v", bitmapFilePath, err)
		}
	}

	//bloques.dat
	bloquesFilePath = pathFS + "/bloques.dat"
	_, err := os.Stat(bloquesFilePath)
	if os.IsNotExist(err) {
		CrearBloques(pathFS, sizeBloques)
	}
}

/*---------------------- FUNCIONES BITMAP ----------------------*/
func CrearBitmap(path string, bitmapSize int) {
	bitmapFilePath := path + "/bitmap.dat"

	bitmapFile, err := os.Create(bitmapFilePath)
	if err != nil {
		log.Fatalf("Error al crear el archivo de bitmap '%s': %v", path, err)
	}
	defer bitmapFile.Close()

	bitmap := NewBitmap()
	bitmapGlobal = bitmap

	/*bitmapBytes := bitmap.ToBytes()
	_, err = bitmapFile.Write(bitmapBytes)
	if err != nil {
		log.Fatalf("Error al inicializar el archivo de bitmap '%s': %v", path, err)
	}
	*/
	time.Sleep(time.Duration(ConfigFS.Block_access_delay) * time.Millisecond)
}

func NewBitmap() *Bitmap {
	return &Bitmap{bits: make([]int, ConfigFS.Block_count)}
}

func (b *Bitmap) ToBytes() []byte {
	bytes := make([]byte, len(b.bits)/8)
	for i, bit := range b.bits {
		bytes[i/8] |= byte(bit) << uint(i%8)
	}
	return bytes
}

func (b *Bitmap) FromBytes(bytes []byte) {
	b.bits = make([]int, len(bytes)*8)
	for i, byteVal := range bytes {
		for j := 0; j < 8; j++ {
			b.bits[i*8+j] = int(byteVal>>uint(j)) & 1
		}
	}
}

func cargarBitmap() error {
    bitmapFile, err := os.Open(bitmapFilePath)
    if err != nil {
        return err
    }
    defer bitmapFile.Close()

    fileInfo, err := bitmapFile.Stat()
    if err != nil {
        return err
    }

    if fileInfo.Size() == 0 {
        // El archivo está vacío, no hay necesidad de cargar el bitmap
		bitmapGlobal = NewBitmap()
        return nil
    }
    bitmapBytes := make([]byte, ConfigFS.Block_count/8)
    _, err = bitmapFile.Read(bitmapBytes)
    if err != nil {
        return err
    }

    bitmapGlobal = NewBitmap()
    bitmapGlobal.FromBytes(bitmapBytes)
    return nil
}

func actualizarBitmap() error {

	bitmapFile, err := os.OpenFile(bitmapFilePath, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer bitmapFile.Close()
	bitmapBytes := bitmapGlobal.ToBytes()
	_, err = bitmapFile.Write(bitmapBytes)
	if err != nil {
		return err
	}

	return nil
}

/*---------------------- FUNCIONES BLOQUES ----------------------*/
func CrearBloques(path string, sizeBloques int) {
	bloquesFilePath := path + "/bloques.dat"
	bloquesFile, err := os.Create(bloquesFilePath)
	if err != nil {
		log.Fatalf("Error al crear el archivo de bloques %s : %v", bloquesFilePath, err)
	}
	defer bloquesFile.Close()
	err2 := bloquesFile.Truncate(int64(sizeBloques))
	if err2 != nil {
		log.Fatalf("Error al establecer tamanio del archivo de bloques %s a tamanio %d: %v", bloquesFilePath, sizeBloques, err)
	}
}

/*---------------------- FUNCIONES DUMP MEMORY ----------------------*/

func DumpMemory(w http.ResponseWriter, r *http.Request) {
	dumpReq := FSmemoriaREQ{}
	var response map[string]bool
	
	// Decodificar la solicitud JSON
	if err := json.NewDecoder(r.Body).Decode(&dumpReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cantidadDeBloques := int(math.Ceil(float64(dumpReq.Tamanio)/float64(ConfigFS.Block_size))) + 1 //ver si se puede mejorar
	// Verificar si hay suficiente espacio
	if !hayEspacioDisponible(cantidadDeBloques) || !entraEnElBloqueDeIndice(cantidadDeBloques-1) {
		log.Printf("No hay suficiente espacio")
		response = map[string]bool{"resultado": false}
	}else {
			response = map[string]bool{"resultado": true}
			// Reservar bloques en el bitmap
			bloquesReservados, err := reservarBloques(cantidadDeBloques, dumpReq.NombreArchivo)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			return
			}
			
			// Crear archivo de metadata
			archivoMetaData, err := crearArchivoMetaData(dumpReq.NombreArchivo, bloquesReservados[0], dumpReq.Tamanio)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			
			log.Printf("## Archivo Creado: <%s> - Tamanio: <%d>", archivoMetaData, dumpReq.Tamanio)
			// Guardar el bitmap actualizado en el archivo
			err = actualizarBitmap()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Escribir contenido a los bloques de datos
			err = actualizarBloques(bloquesReservados, dumpReq.Data)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
	}
	respBytes, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(respBytes)
}

func actualizarBloques(bloquesReservados []int, dumpReqData []byte) error {
	bloquesFile, err := os.OpenFile(bloquesFilePath, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer bloquesFile.Close()
	cargarBloqueIndices(bloquesFile, bloquesReservados)
	bloques := bloquesReservados[1:]
	var data []byte
	for indice, bloqueIndex := range bloques {
		if (indice+1)*ConfigFS.Block_size < len(dumpReqData) {
			data = dumpReqData[indice*ConfigFS.Block_size : (indice+1)*ConfigFS.Block_size]
		} else {
			data = dumpReqData[indice*ConfigFS.Block_size:]
		}
		escribirBloque(bloqueIndex, bloquesFile, data)
	}

	return nil
}

func cargarBloqueIndices(bloquesFile *os.File, bloquesReservados []int) {
	bloqueIndex := bloquesReservados[0]
	bloquesABytes := toBytesBloque(bloquesReservados[1:])
	escribirBloque(bloqueIndex, bloquesFile, bloquesABytes)

}
func escribirBloque(bloque int, bloquesFile *os.File, data []byte) {
	bloquesFile.Seek(int64(bloque*ConfigFS.Block_size), 0)
	_, err := bloquesFile.Write(data)
	if err != nil {
		log.Fatalf("Error al escribir el bloque de indices. Bloque numero: %d ", bloque)
	}
	time.Sleep(time.Duration(ConfigFS.Block_access_delay) * time.Millisecond)
}
func toBytesBloque(bloque []int) []byte {
	byteSlice := make([]byte, len(bloque))
	for i, v := range bloque {
		byteSlice[i] = byte(v)
	}
	return byteSlice
}

func entraEnElBloqueDeIndice(cantidadDeBloques int) bool {
	return cantidadDeBloques*4 <= ConfigFS.Block_size
}

func hayEspacioDisponible(cantidadDeBloques int) bool {
	freeBlocks := 0
	for _, block := range bitmapGlobal.bits {
		if block == 0 { // Si el bloque está libre
			freeBlocks++
		}
		if freeBlocks >= cantidadDeBloques {
			return true
		}
	}
	return false
}

// Función para reservar bloques en el bitmapGlobal.bits
func reservarBloques(cantidadDeBloques int, nombreArchivo string) ([]int, error) {
	bloquesAsignados := []int{}
	for i, block := range bitmapGlobal.bits {
		if block == 0 { // Bloque libre
			bitmapGlobal.bits[i] = 1 // Marcar como ocupado
			bloquesLibres := calcularBloquesLibres(bitmapGlobal.bits)
			log.Printf("## Bloque asignado: <%d> - Archivo: <%s> - Bloques Libres: <%d>", i, nombreArchivo, bloquesLibres)
			bloquesAsignados = append(bloquesAsignados, i)
		}
		if len(bloquesAsignados) == cantidadDeBloques {
			return bloquesAsignados, nil
		}
	}
	return nil, fmt.Errorf("no hay suficiente espacio para el archivo") //?????
}

func calcularBloquesLibres(bitmap []int) int {
	bloquesLibres := 0
	for _, block := range bitmap {
		if block == 0 {
			bloquesLibres++
		}
	}
	return bloquesLibres
}

func crearArchivoMetaData(filename string, indexBlock int, size uint32) (string, error) {
	// Crear archivo de metadata en la carpeta /files
	file, err := os.Create(fmt.Sprintf("%s/%s", ConfigFS.Mount_dir, filename))
	if err != nil {
		return "", fmt.Errorf("error al crear archivo de metadata: %w", err)
	}
	defer file.Close()

	// Crear estructura de metadata
	metadata := Metadata{
		IndexBlock: indexBlock,
		Size:       size,
	}

	// Escribir metadata en el archivo en formato JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Para mejorar la legibilidad del JSON
	err = encoder.Encode(metadata)
	if err != nil {
		return "", fmt.Errorf("error al escribir metadata: %w", err)
	}

	return filename, nil
}