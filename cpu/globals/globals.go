package globals

type Config struct {
	IpMemoria     string `json:"ip_memoria"`     //IP a la cual se deberá conectar con la Memoria
	PuertoMemoria int    `json:"puerto_memoria"` //Puerto al cual se deberá conectar con la Memoria
	IpKernel      string `json:"ip_kernel"`      //IP a la cual se deberá conectar con el Kernel
	PuertoKernel  int    `json:"puerto_kernel"`  //Puerto al cual se deberá conectar con el Kernel
	Puerto        int    `json:"puerto"`         //Puerto en el cual se deberá escuchar las conexiones
	LogLevel      string `json:"log_level"`      //Nivel de detalle máximo a mostrar.

}

type Paquete struct {
	ID      string `json:"ID"` //de momento es un string que indica desde donde sale el mensaje.
	Mensaje string `json:"mensaje"`
	Size    int16  `json:"size"`
	Array   []rune `json:"array"`
}

var ClientConfig *Config
