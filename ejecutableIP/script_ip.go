package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Función para modificar los campos en un archivo JSON
func modifyJSONFile(filePath string, FS string, CPU string, kernel string, memoria string) error {
	// Leer el contenido del archivo
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error al leer el archivo %s: %v", filePath, err)
	}

	// Deserializar el JSON en un mapa genérico
	var content map[string]interface{}
	if err := json.Unmarshal(data, &content); err != nil {
		return fmt.Errorf("error al deserializar JSON en %s: %v", filePath, err)
	}

	// Modificar los campos solo si existen
	if _, exists := content["ip_filesystem"]; exists && FS != "" {
		content["ip_filesystem"] = FS
	}
	if _, exists := content["ip_cpu"]; exists && CPU != "" {
		content["ip_cpu"] = CPU
	}
	if _, exists := content["ip_kernel"]; exists && kernel != "" {
		content["ip_kernel"] = kernel
	}
	if _, exists := content["ip_memoria"]; exists && memoria != "" {
		content["ip_memoria"] = memoria
	}

	// Serializar el mapa modificado a JSON
	modifiedData, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return fmt.Errorf("error al serializar JSON en %s: %v", filePath, err)
	}

	// Guardar el archivo modificado
	if err := ioutil.WriteFile(filePath, modifiedData, 0644); err != nil {
		return fmt.Errorf("error al guardar el archivo %s: %v", filePath, err)
	}

	return nil
}

func main() {
	// Parsear argumentos de la línea de comandos
	FS := flag.String("fs", "", "Valor para ip_filesystem")
	CPU := flag.String("cpu", "", "Valor para ip_cpu")
	Kernel := flag.String("kernel", "", "Valor para ip_kernel")
	Memoria := flag.String("memoria", "", "Valor para ip_memoria")
	flag.Parse()

	// Directorios de archivos JSON a modificar
	jsonDirs := []string{"./cpu/configsCPU", "./memoria/configsMemoria",
		"./kernel/configsKERNEL", "./filesystem/configsFS"}

	// jsonDirs := []string{"C:/Users/faust/Documents/UTN/tp-2024-1c-Panza_confianza/cpu/CPUconfigs"}
	// Verificar que se hayan proporcionado todos los argumentos necesarios
	if *FS == "" || *CPU == "" || *Kernel == "" || *Memoria == "" {
		fmt.Println("Todos los parametros (fs, cpu, kernel, memoria) son obligatorios.")
		flag.Usage()
		os.Exit(1)
	}

	for _, dir := range jsonDirs {
		// Buscar archivos JSON en el directorio
		files, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			fmt.Printf("Error al buscar archivos JSON en el directorio %s: %v\n", dir, err)
			continue
		}

		for _, filePath := range files {
			if err := modifyJSONFile(filePath, *FS, *CPU, *Kernel, *Memoria); err != nil {
				fmt.Printf("Error al modificar el archivo %s: %v\n", filePath, err)
			}
		}
	}
}
