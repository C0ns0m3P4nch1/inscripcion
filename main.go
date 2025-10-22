package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/textproto"
    "mime/multipart"
    "os"
    "sync"
	//"bufio"
    "strconv"
    //"runtime"
    "strings"
    "time"
)

// === CLIENTE HTTP OPTIMIZADO ===
var httpClient = &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100000,
        MaxIdleConnsPerHost: 100000,
        MaxConnsPerHost:     0,
        IdleConnTimeout:     0,
        DisableKeepAlives:   false,
        ForceAttemptHTTP2:   true, // permite HTTP/2 si el server lo soporta
    },
}

var (
    usuarios     []string
    indiceActual int
)

// === CONTEXTO ===
type Context struct {
    BaseURL    string
    Token      string
    Documento  string
    Correo     string
    Radicado   int
    Referencia string
}

// === MAIN ===
func main() {
    inicializarUsuariosDesdeArchivo()
    
    numUsuarios := 1000   // cuántos usuarios quieres simular
    // numWorkers ya no es necesario en este enfoque; crearemos N goroutines
    startCh := make(chan struct{})   // puerta de inicio
    var doneWG sync.WaitGroup
    doneWG.Add(numUsuarios)

    // preparar lista de documentos (puedes leer desde archivo o generar)
    documentos := make([]string, 0, numUsuarios)
    for i := 0; i < numUsuarios; i++ {
        documentos = append(documentos, siguienteUsuario()) // o genera
    }

    // crear goroutines (preparadas, esperando startCh)
    for i := 0; i < numUsuarios; i++ {
        doc := documentos[i]
        go func(documento string, id int) {
            defer doneWG.Done()
            correo := fmt.Sprintf("prueba_carga%s@yopmail.com", documento)
            ctx := &Context{
                BaseURL:   "https://d392rp1p6w2dkx.draitest.com",
                Documento: documento,
                Correo:    correo,
            }

            // Espera la señal de inicio (start gate)
            <-startCh

            // Ejecuta el flujo (esto será lanzado "casi" simultáneamente)
            if err := ejecutarFlujo(ctx); err != nil {
                // Aquí puedes loguear a un canal o contador en vez de imprimir (evita mucho I/O)
                fmt.Printf("[Error] usuario %s: %v\n", documento, err)
            }
        }(doc, i+1)
    }

    // --- medición ---
    t0 := time.Now()
    close(startCh)       // libera a todas las goroutines a la vez
    doneWG.Wait()
    elapsed := time.Since(t0)
    fmt.Printf("Completadas %d ejecuciones en %v\n", numUsuarios, elapsed)
}

// func main() {
//     if len(os.Args) < 2 {
//         fmt.Println("Uso: go run main.go <ruta-archivo-cedulas>")
//         return
//     }

//     filePath := os.Args[1]
//     fmt.Println("[+] Archivo de cédulas:", filePath)

//     // === 🔥 Usa los 8 núcleos detectados ===
//     numCPU := runtime.NumCPU()
//     runtime.GOMAXPROCS(numCPU)
//     fmt.Printf("[+] Núcleos detectados: %d — todos en uso\n", numCPU)

//     // Ajuste de hilos simultáneos
//     // ⚠️ No uses miles, el límite real de rendimiento está entre 500–2000 dependiendo del servidor y tu red.
//     threads := numCPU * 250 // 8 núcleos × 250 = 2000 goroutines simultáneas aprox.
//     baseURL := "https://d392rp1p6w2dkx.draitest.com"

//     // Abrimos el archivo de cédulas
//     file, err := os.Open(filePath)
//     if err != nil {
//         fmt.Println("Error abriendo archivo:", err)
//         return
//     }
//     defer file.Close()

//     scanner := bufio.NewScanner(file)
//     var wg sync.WaitGroup
//     ch := make(chan struct{}, threads) // canal de control de concurrencia

//     start := time.Now()
//     i := 0

//     for scanner.Scan() {
//         documento := strings.TrimSpace(scanner.Text())
//         if documento == "" {
//             continue
//         }

//         wg.Add(1)
//         ch <- struct{}{} // bloquea si ya hay 'threads' activos

//         go func(id int, doc string) {
//             defer wg.Done()
//             defer func() { <-ch }()

//             // 🔥 Ejecuta el flujo completo en paralelo real
//             correo := fmt.Sprintf("prueba_carga%s@yopmail.com", doc)
//             ctx := &Context{
//                 BaseURL:   baseURL,
//                 Documento: doc,
//                 Correo:    correo,
//             }

//             _ = ejecutarFlujo(ctx)
//         }(i, documento)
//         i++
//     }

//     if err := scanner.Err(); err != nil {
//         fmt.Println("Error leyendo archivo:", err)
//     }

//     wg.Wait()
//     elapsed := time.Since(start)

//     fmt.Printf("\n=== Completadas %d ejecuciones en %.2f segundos usando %d hilos reales (%d núcleos) ===\n",
//         i, elapsed.Seconds(), threads, numCPU)
// }



// === FLUJO GENERAL ===
func ejecutarFlujo(ctx *Context) error {
    pasos := []func(*Context) error{
        paso1VerificarPreinscripcion,
        paso2VerificarPreinscripcionCorreo,
        paso3RegistrarPreinscripcion,
        paso4Autenticacion,
        func(c *Context) error { _, err := paso5ObtenerInscripcion(c); return err },
        func(c *Context) error { _, err := paso6ObtenerPago(c); return err },
        func(c *Context) error { _, err := paso7ObtenerDepartamentos(c); return err },
        func(c *Context) error { _, err := paso8ObtenerSitiosEvaluacion(c); return err },
        func(c *Context) error { _, err := paso9ObtenerEstablecimientos(c); return err },
        paso10RegistrarPago,
        paso11RegistrarInscripcion,
        paso12ModificarInscripcion,
    }
    for _, fn := range pasos {
        if err := fn(ctx); err != nil {
            return err
        }
    }
    return nil
}

// === Paso 1 === # OPTIMIZADO #
func paso1VerificarPreinscripcion(ctx *Context) error {
    url := fmt.Sprintf("%s/inscripciones/inscripcion/rc/verificar-preinscripcion?idConcurso=1&documento=%s", ctx.BaseURL, ctx.Documento)
    return ejecutarGET(ctx, url, 1)
}

// === Paso 2 === # OPTIMIZADO #
func paso2VerificarPreinscripcionCorreo(ctx *Context) error {
    url := fmt.Sprintf("%s/inscripciones/inscripcion/rc/verificar-preinscripcion?idConcurso=1&documento=%s&correo=%s", ctx.BaseURL, ctx.Documento, ctx.Correo)
    return ejecutarGET(ctx, url, 2)
}

// === Paso 3 === # OPTIMIZADO #
func paso3RegistrarPreinscripcion(ctx *Context) error {
    url := ctx.BaseURL + "/inscripciones/inscripcion/rc/preinscripcion"

    // Template JSON hardcodeado (reutilizable sin marshal)
    jsonTemplate := `{
        "persona": {
            "documento": "%s",
            "email": "%s",
            "primerNombre": "PEDRO",
            "segundoNombre": "PABLO",
            "primerApellido": "GALLEGO",
            "segundoApellido": "PINZON",
            "tipoDocumento": "CC"
        },
        "fechaAceptacionTerminos": "2025-10-15T20:55:00.000Z",
        "idConcurso": 1,
        "idNivel": 1,
        "idTipoConcurso": 1,
        "contrasena": "*05310914Prueba"
    }`

    jsonBody := fmt.Sprintf(jsonTemplate, ctx.Documento, ctx.Correo)

    req, err := http.NewRequest("POST", url, strings.NewReader(jsonBody))
    if err != nil {
        return fmt.Errorf("error creando request paso 3: %w", err)
    }

    setDefaultHeaders(req)
    req.Header.Set("Content-Type", "application/json")

    resp, err := httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("error ejecutando request paso 3: %w", err)
    }
    defer resp.Body.Close()

    data, _ := io.ReadAll(resp.Body)

    if resp.StatusCode != 200 {
        return fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
    }

    // Extraer radicado sin unmarshalling completo
    if idx := bytes.Index(data, []byte(`"radicado":`)); idx > 0 {
        var num int
        fmt.Sscanf(string(data[idx:]), `"radicado":%d`, &num)
        ctx.Radicado = num
    }

    return nil
}


// Paso 4: Autenticación # OPTIMIZADO #
func paso4Autenticacion(ctx *Context) error {
    url := ctx.BaseURL + "/inscripciones/sistema/rc/autenticacion"

    body := map[string]interface{}{
        "radicado":   ctx.Radicado,
        "contrasena": "*05310914Prueba",
    }

    jsonBody, err := json.Marshal(body)
    if err != nil {
        return fmt.Errorf("error serializando cuerpo paso 4: %v", err)
    }

    req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonBody))
    if err != nil {
        return fmt.Errorf("error creando request paso 4: %v", err)
    }
    setDefaultHeaders(req)
    req.Header.Set("Content-Type", "application/json")
    resp, err := httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("error ejecutando request paso 4: %v", err)
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()
    if resp.StatusCode != http.StatusOK {
        bodyResp, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyResp))
    }
    var respData struct {
        Token string `json:"token"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
        return fmt.Errorf("error parseando respuesta JSON paso 4: %v", err)
    }
    if respData.Token == "" {
        return fmt.Errorf("respuesta sin token válido")
    }
    ctx.Token = respData.Token
    return nil
}


// === Paso 5 === # OPTIMIZADO #
func paso5ObtenerInscripcion(ctx *Context) (string, error) {
    url := ctx.BaseURL + "/inscripciones/inscripcion/actual"
    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        return "", fmt.Errorf("error creando request paso 5: %v", err)
    }
    setDefaultHeaders(req)
    req.Header.Set("Authorization", "bearer "+ctx.Token)
    resp, err := httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error ejecutando request paso 5: %v", err)
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()
    if resp.StatusCode != http.StatusOK {
        bodyResp, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyResp))
    }
    data, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error leyendo respuesta paso 5: %v", err)
    }
    return string(data), nil
}


// === Paso 6 === # OPTIMIZADO #
func paso6ObtenerPago(ctx *Context) (string, error) {
    url := ctx.BaseURL + "/inscripciones/pago"

    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        return "", fmt.Errorf("error creando request paso 6: %v", err)
    }
    setDefaultHeaders(req)
    req.Header.Set("Authorization", "bearer "+ctx.Token)

    resp, err := httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error ejecutando request paso 6: %v", err)
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()

    if resp.StatusCode != http.StatusOK {
        bodyResp, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyResp))
    }

    var respData struct {
        Referencia string `json:"referencia"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
        return "", fmt.Errorf("error parseando respuesta paso 6: %v", err)
    }

    if respData.Referencia == "" {
        return "", fmt.Errorf("no se encontró la referencia en la respuesta del servidor")
    }
    ctx.Referencia = respData.Referencia
    return respData.Referencia, nil
}


// === Paso 7 === # OPTIMIZADO #
func paso7ObtenerDepartamentos(ctx *Context) (string, error) {
    url := ctx.BaseURL + "/inscripciones/publico/dane/departamentos"

    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        return "", fmt.Errorf("error creando request paso 7: %v", err)
    }

    setDefaultHeaders(req)

    resp, err := httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error ejecutando request paso 7: %v", err)
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()
    if resp.StatusCode != http.StatusOK {
        bodyResp, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyResp))
    }
    bodyBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error leyendo respuesta paso 7: %v", err)
    }
    return string(bodyBytes), nil
}


// === Paso 8 === # OPTIMIZADO #
func paso8ObtenerSitiosEvaluacion(ctx *Context) (string, error) {
    url := ctx.BaseURL + "/inscripciones/publico/dane/sitios-evaluacion"
    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        return "", fmt.Errorf("error creando request paso 8: %v", err)
    }
    setDefaultHeaders(req)
    resp, err := httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error ejecutando request paso 8: %v", err)
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()
    if resp.StatusCode != http.StatusOK {
        bodyResp, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyResp))
    }
    bodyBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error leyendo respuesta paso 8: %v", err)
    }
    return string(bodyBytes), nil
}

// === Paso 9 === # OPTIMIZADO #
func paso9ObtenerEstablecimientos(ctx *Context) (string, error) {
    url := ctx.BaseURL + "/inscripciones/publico/dane/establecimientos?codigoMunicipio=5001"

    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        return "", fmt.Errorf("error creando request paso 9: %v", err)
    }

    setDefaultHeaders(req)

    resp, err := httpClient.Do(req)
    if err != nil {
        return "", fmt.Errorf("error ejecutando request paso 9: %v", err)
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()

    if resp.StatusCode != http.StatusOK {
        bodyResp, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyResp))
    }

    bodyBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error leyendo respuesta paso 9: %v", err)
    }

    return string(bodyBytes), nil
}


// === Paso 10 === # OPTIMIZADO #
func paso10RegistrarPago(ctx *Context) error {
    url := ctx.BaseURL + "/inscripciones/pago/confirmar/" + ctx.Referencia

    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        return fmt.Errorf("error creando request paso 10: %v", err)
    }

    req.Header.Set("token", "Gr$1yN0mTNd6o|0bh8(=")

    resp, err := httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("error ejecutando request paso 10: %v", err)
    }
    defer func() {
        io.Copy(io.Discard, resp.Body)
        resp.Body.Close()
    }()

    if resp.StatusCode != http.StatusOK {
        bodyResp, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("error paso 10, status %d: %s", resp.StatusCode, string(bodyResp))
    }

    return nil
}


// === Paso 11 ===
func paso11RegistrarInscripcion(ctx *Context) error {
    docNum, err := strconv.Atoi(ctx.Documento)
    if err != nil {
        return fmt.Errorf("documento no es un número válido: %v", err)
    }

    pdfHardcodeado := []byte(`%PDF-1.4
% ... contenido mínimo de PDF ...
%%EOF`)

    var body bytes.Buffer
    writer := multipart.NewWriter(&body)

    // PDF solo en 10% de los casos
    if docNum%10 == 0 {
        h := make(textproto.MIMEHeader)
        h.Set("Content-Disposition", `form-data; name="certificadoDiscapacidad"; filename="anexo.pdf"`)
        h.Set("Content-Type", "application/pdf")
        fw, err := writer.CreatePart(h)
        if err != nil {
            return fmt.Errorf("error creando form file PDF: %v", err)
        }
        _, err = fw.Write(pdfHardcodeado)
        if err != nil {
            return fmt.Errorf("error copiando PDF: %v", err)
        }
    }

    // JSON de inscripción
    inscripcionJSON := fmt.Sprintf(`{
    "radicado": %d,
    "persona": {
        "id": 106594,
        "tipoDocumento": "CC",
        "documento": "%s",
        "primerNombre": "Andres",
        "segundoNombre": "Camilo",
        "primerApellido": "Osorno",
        "segundoApellido": "Zapata",
        "email": "%s",
        "inconsistencias": [],
        "ciudad": {
            "id": 5030,
            "nombre": "Amagá",
            "departamento": {
                "id": 5,
                "nombre": "Antioquia"
            }
        },
        "telefono1": "3207775032",
        "telefono2": "3207775032",
        "fechaNacimiento": "1990-06-30T05:00:00.000Z",
        "sexo": "M",
        "tieneDiscapacidad": "SI",
        "tipoDiscapacidad": "Sensorial - Ceguera",
        "necesitaAsistenciaParaPruebas": "NO"
    },
    "estadoInscrito": {
        "id": 1,
        "nombreEstado": "Preinscrito"
    },
    "concurso": {
        "id": 1
    },
    "convocatoria": {
        "id": 0,
        "esActiva": false
    },
    "borrado": false,
    "tieneCitaciones": false,
    "causalesInadmision": [],
    "inconsistencias": [],
    "cargo": "Docente de aula",
    "nivelDesempeno": "Media",
    "areaDesempeno": "MATTTES",
    "fechaVinculacion": "2012-11-23T05:00:00.000Z",
    "tiempoServicioMeses": 98,
    "numeroMovimientos": 1,
    "nivelEducativo": "Especialización",
    "nivelEducativoEtc": "Maestría",
    "formacionAcademica": "Licenciado",
    "detalleFormacionAcademica": "Matematicas",
    "gradoEscalafonActual": 1,
    "nivelEscalafonActual": "A",
    "gradoEscalafonAspira": 2,
    "nivelEscalafonAspira": "A",
    "sede": {
        "codigo": 105001000001,
        "nombre": "INSTITUCIÓN EDUCATIVA FE Y ALEGRIA JOSÉ MARÍA VELAZ",
        "secretaria": "MEDELLIN",
        "daneEstablecimiento": {
            "codigo": 105001000001,
            "nombre": "INSTITUCIÓN EDUCATIVA FE Y ALEGRIA JOSÉ MARIA VELAZ"
        },
        "municipioSede": {
            "id": 5001,
            "nombre": "Medellín",
            "departamento": {
                "id": 5,
                "nombre": "Antioquia"
            }
        }
    },
    "sitioEvaluacion": {
        "id": 5615,
        "nombre": "Rionegro\r\n",
        "departamento": {
            "id": 5,
            "nombre": "Antioquia"
        }
    },
    "pruebaPedagogicaAplicar": "Docente de aula preescolar",
    "fechaAceptacionLugarPrueba": "2025-10-16T21:02:14.771Z",
    "fechaAceptacionTerminosInscripcion": "1970-01-01T00:00:00.000Z",
    "fechaInscripcion": "2025-10-16T21:03:51.566Z"
}`, ctx.Radicado, ctx.Documento, ctx.Correo)

    h := make(textproto.MIMEHeader)
    h.Set("Content-Disposition", `form-data; name="inscripcion"; filename="blob"`)
    h.Set("Content-Type", "application/json")
    fw, err := writer.CreatePart(h)
    if err != nil {
        return fmt.Errorf("error creando form file JSON: %v", err)
    }
    _, err = fw.Write([]byte(inscripcionJSON))
    if err != nil {
        return fmt.Errorf("error copiando JSON: %v", err)
    }

    writer.Close()

    req, err := http.NewRequest("POST", ctx.BaseURL+"/inscripciones/inscripcion", &body)
    if err != nil {
        return fmt.Errorf("error creando request: %v", err)
    }

    req.Header.Set("Authorization", "bearer "+ctx.Token)
    req.Header.Set("Content-Type", writer.FormDataContentType())

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("error ejecutando request: %v", err)
    }
    defer resp.Body.Close()

    //respBody, _ := io.ReadAll(resp.Body)

    if resp.StatusCode != 200 {
        return fmt.Errorf("error paso 11, status: %d", resp.StatusCode)
    }

    return nil
}

// === Paso 12 ===
func paso12ModificarInscripcion(ctx *Context) error {
    docNum, err := strconv.Atoi(ctx.Documento)
    if err != nil {
        return fmt.Errorf("documento no es un número válido: %v", err)
    }

    pdfHardcodeado := []byte(`%PDF-1.4
% ... contenido mínimo de PDF ...
%%EOF`)

    var body bytes.Buffer
    writer := multipart.NewWriter(&body)

    // Enviar PDF solo en 10% de los casos
    if docNum%10 == 0 {
        h := make(textproto.MIMEHeader)
        h.Set("Content-Disposition", `form-data; name="certificadoDiscapacidad"; filename="anexo.pdf"`)
        h.Set("Content-Type", "application/pdf")
        fw, err := writer.CreatePart(h)
        if err != nil {
            return fmt.Errorf("error creando form file PDF: %v", err)
        }
        _, err = fw.Write(pdfHardcodeado)
        if err != nil {
            return fmt.Errorf("error copiando PDF: %v", err)
        }
    }

    // JSON de modificación de inscripción
    inscripcionJSON := fmt.Sprintf(`{
        "radicado": %d,
        "persona": {
            "id": 106569,
            "tipoDocumento": "CC",
            "documento": "%s",
            "primerNombre": "PEDRO",
            "segundoNombre": "PEDRO PABLO",
            "primerApellido": "GALLEGO",
            "segundoApellido": "GALLEGO PINZON",
            "email": "%s",
            "inconsistencias": [],
            "ciudad": {"id":11001,"nombre":"Bogotá D.C.","departamento":{"id":11,"nombre":"Bogotá D.C."}},
            "telefono1": "3112163766",
            "telefono2": null,
            "fechaNacimiento": "2007-09-01T05:00:00.000Z",
            "sexo": "M",
            "tieneDiscapacidad": "SI",
            "tipoDiscapacidad": "Sensorial - Sordera Profunda,Sensorial - Hipoacusia",
            "necesitaAsistenciaParaPruebas": "SI",
            "tipoAsistenciaParaPruebas": "Apoyo de accesibilidad"
        },
        "estadoInscrito":{"id":1,"nombreEstado":"Preinscrito"},
        "concurso":{"id":1},
        "convocatoria":{"id":1,"codigoConvocatoria":"01-0001","denominacion":"profesional ascenso","esActiva":false},
        "borrado": false,
        "tieneCitaciones": false,
        "causalesInadmision": [],
        "inconsistencias": [],
        "cargo": "Docente de aula",
        "nivelDesempeno": "Preescolar",
        "areaDesempeno": "YYYY",
        "fechaVinculacion": "2025-09-25T05:00:00.000Z",
        "tiempoServicioMeses": 52,
        "numeroMovimientos": 4,
        "nivelEducativo": "Pregrado",
        "nivelEducativoEtc": "No aplica",
        "formacionAcademica": "Profesional no licenciado",
        "detalleFormacionAcademica": "ddddd",
        "gradoEscalafonActual": 1,
        "nivelEscalafonActual": "B",
        "gradoEscalafonAspira": 1,
        "nivelEscalafonAspira": "C",
        "sede": {
            "codigo": 105001000001,
            "nombre": "INSTITUCIÓN EDUCATIVA FE Y ALEGRIA JOSÉ MARÍA VELAZ",
            "secretaria": "MEDELLIN",
            "daneEstablecimiento": {
                "codigo": 105001000001,
                "nombre": "INSTITUCIÓN EDUCATIVA FE Y ALEGRIA JOSÉ MARIA VELAZ"
            },
            "municipioSede": {
                "id": 5001,
                "nombre": "Medellín",
                "departamento": {"id": 5, "nombre": "Antioquia"}
            }
        },
        "sitioEvaluacion": {
            "id": 11001,
            "nombre": "Bogotá D.C.",
            "departamento": {"id": 11, "nombre": "Bogotá D.C."}
        },
        "pruebaPedagogicaAplicar": "Coordinador (a)",
        "fechaAceptacionLugarPrueba": "2025-09-26T19:58:41.756Z",
        "fechaAceptacionTerminosInscripcion": "1970-01-01T00:00:00.000Z",
        "fechaInscripcion": "2025-09-26T19:59:04.868Z"
    }`, ctx.Radicado, ctx.Documento, ctx.Correo)

    // Parte JSON
    h := make(textproto.MIMEHeader)
    h.Set("Content-Disposition", `form-data; name="inscripcion"; filename="blob"`)
    h.Set("Content-Type", "application/json")
    fw, err := writer.CreatePart(h)
    if err != nil {
        return fmt.Errorf("error creando form file JSON: %v", err)
    }
    _, err = fw.Write([]byte(inscripcionJSON))
    if err != nil {
        return fmt.Errorf("error copiando JSON: %v", err)
    }

    writer.Close()

    // Request HTTP
    req, err := http.NewRequest("POST", ctx.BaseURL+"/inscripciones/inscripcion/actualizacion", &body)
    if err != nil {
        return fmt.Errorf("error creando request: %v", err)
    }

    req.Header.Set("Authorization", "bearer "+ctx.Token)
    req.Header.Set("Content-Type", writer.FormDataContentType())

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("error ejecutando request: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return fmt.Errorf("error paso 12, status: %d", resp.StatusCode)
    }

    fmt.Printf("Listo Usuario %s, Resultado: %d \n", ctx.Documento, resp.StatusCode)

    return nil
}

func ejecutarGET(ctx *Context, url string, paso int) error {
    return nil
}

func ejecutarConAutorizacion(ctx *Context, req *http.Request, paso int) error {
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    data, _ := io.ReadAll(resp.Body)
    fmt.Printf("[✓] Paso %d completado (status: %d)\n", paso, resp.StatusCode)
    if resp.StatusCode != 200 {
        return fmt.Errorf("status inesperado: %d", resp.StatusCode)
    }

    if strings.Contains(string(data), "token expirado") {
        return fmt.Errorf("token expirado en paso %d", paso)
    }

    return nil
}

func setDefaultHeaders(req *http.Request) {
    req.Header.Set("Accept", "application/json, text/plain, */*")
    req.Header.Set("Connection", "keep-alive")
}

// Cargar todas las cédulas desde un archivo que se pasa como argumento al ejecutar
func inicializarUsuariosDesdeArchivo() {
    if len(os.Args) < 2 {
        fmt.Println("Uso: go run main.go <ruta_archivo_usuarios>")
        os.Exit(1)
    }

    archivo := os.Args[1]

    contenido, err := os.ReadFile(archivo)
    if err != nil {
        fmt.Printf("Error leyendo archivo '%s': %v\n", archivo, err)
        os.Exit(1)
    }

    lineas := strings.Split(strings.ReplaceAll(string(contenido), "\r\n", "\n"), "\n")
    for _, l := range lineas {
        l = strings.TrimSpace(l)
        if l != "" {
            usuarios = append(usuarios, l)
        }
    }

    if len(usuarios) == 0 {
        fmt.Println("El archivo no contiene cédulas válidas.")
        os.Exit(1)
    }

    fmt.Printf("[+] Cargadas %d cédulas desde %s\n", len(usuarios), archivo)
}

// Retorna la siguiente cédula del archivo
func siguienteUsuario() string {
    if indiceActual >= len(usuarios) {
        fmt.Println("[!] No hay más cédulas en el archivo.")
        os.Exit(0)
    }

    doc := usuarios[indiceActual]
    indiceActual++
    return doc
}