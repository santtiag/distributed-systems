"use client";

import { useState, useEffect } from "react";

interface ImageMetrics {
    image_id: string;
    original_url: string;
    download_status: string;
    resize_status: string;
    convert_status: string;
    watermark_status: string;
    download_time_sec: number;
    resize_time_sec: number;
    convert_time_sec: number;
    watermark_time_sec: number;
    download_worker: string;
    resize_worker: string;
    convert_worker: string;
    watermark_worker: string;
}

interface StageMetrics {
    total_processed: number;
    total_failed: number;
    total_time_sec: number;
    avg_time_sec: number;
}

interface JobData {
    job_info: {
        job_id: string;
        status: string;
        total_files: number;
        processed_files: number;
        failed_files: number;
        start_time: string;
        end_time: string | null;
        images: ImageMetrics[];
    };
    global_metrics: {
        total_download_time_sec: number;
        total_resize_time_sec: number;
        total_convert_time_sec: number;
        total_watermark_time_sec: number;
        total_execution_time_sec: number;
        success_percentage: number;
        failure_percentage: number;
    };
    stage_metrics: {
        download: StageMetrics;
        resize: StageMetrics;
        convert: StageMetrics;
        watermark: StageMetrics;
    };
}

export default function Home() {
    // Estados para Creación (Comando)
    const [urlsInput, setUrlsInput] = useState("");
    const [workersDownload, setWorkersDownload] = useState(1);
    const [workersResize, setWorkersResize] = useState(1);
    const [workersConvert, setWorkersConvert] = useState(1);
    const [workersWatermark, setWorkersWatermark] = useState(1);
    const [errorV, setErrorV] = useState("");

    // Estados para Búsqueda (Query)
    const [searchId, setSearchId] = useState("");
    const [searchError, setSearchError] = useState("");

    // Estados compartidos (Dashboard)
    const [jobId, setJobId] = useState<string | null>(null);
    const [jobData, setJobData] = useState<JobData | null>(null);

    // Acción 1: Crear nuevo procesamiento (POST :3000)
    const submitPipeline = async () => {
        setErrorV("");
        const urls = urlsInput.split("\n").map(u => u.trim()).filter(u => u !== "");

        if (urls.length === 0) {
            setErrorV("INGRESE AL MENOS UNA URL VALIDA");
            return;
        }

        try {
            const res = await fetch("http://localhost:5000/api/v1/process", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                    urls,
                    workers_download: workersDownload,
                    workers_resize: workersResize,
                    workers_convert: workersConvert,
                    workers_watermark: workersWatermark
                })
            });

            if (!res.ok) throw new Error("Fallo en la comunicacion con nodo de procesamiento");

            const data = await res.json();
            setJobData(null); // Limpiar dashboard anterior
            setJobId(data.job_id); // Esto dispara el useEffect
            setSearchId(""); // Limpiamos el input de búsqueda manual para evitar confusión
        } catch (err: any) {
            setErrorV(err.message);
        }
    };

    // Acción 2: Consultar procesamiento existente (GET :3001)
    const searchExistingJob = () => {
        setSearchError("");
        if (!searchId.trim()) {
            setSearchError("INGRESE UN UUID VALIDO");
            return;
        }

        setJobData(null); // Limpiar dashboard
        setJobId(searchId.trim()); // Dispara el useEffect hacia el nuevo ID
    };

    // Efecto reactivo: Se encarga de hacer el polling si hay un jobId activo
    useEffect(() => {
        if (!jobId) return;

        let isSubscribed = true;

        const fetchTelemetry = async () => {
            try {
                const res = await fetch(`http://localhost:3001/api/v1/status/${jobId}`);
                if (res.ok) {
                    const data = await res.json();
                    if (isSubscribed) {
                        setJobData(data);
                        setSearchError(""); // Limpiar error si se encontró
                    }
                } else if (res.status === 404) {
                    if (isSubscribed) {
                        setSearchError("IDENTIFICADOR NO ENCONTRADO EN LA BASE DE DATOS");
                        setJobId(null);
                    }
                }
            } catch (err) {
                console.error("Error obteniendo telemetria", err);
            }
        };

        fetchTelemetry();

        // Polling cada 1 segundo si no ha terminado
        const interval = setInterval(() => {
            if (jobData?.job_info?.status !== "COMPLETADO" && jobData?.job_info?.status !== "COMPLETADO_CON_ERRORES" && jobData?.job_info?.status !== "FALLIDO") {
                fetchTelemetry();
            }
        }, 1000);

        return () => {
            isSubscribed = false;
            clearInterval(interval);
        };
    }, [jobId, jobData?.job_info?.status]);

    const formatDateTime = (isoString: string | null) => {
        if (!isoString) return "N/A";
        const date = new Date(isoString);
        return date.toLocaleString("es-ES", {
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit"
        });
    };

    const formatDuration = (seconds: number) => {
        if (seconds < 60) {
            return `${seconds.toFixed(2)}s`;
        }
        const mins = Math.floor(seconds / 60);
        const secs = seconds % 60;
        return `${mins}m ${secs.toFixed(0)}s`;
    };

    const renderStatusBox = (status: string) => {
        let colorClass = "bg-neutral-800 text-neutral-400 border-neutral-700";
        if (status === "PROCESSING") colorClass = "bg-blue-900 text-blue-300 border-blue-500 animate-pulse";
        if (status === "COMPLETADO") colorClass = "bg-emerald-900 text-emerald-300 border-emerald-500";
        if (status === "FALLIDO") colorClass = "bg-red-900 text-red-300 border-red-500";
        if (status === "EN_PROCESO") colorClass = "bg-amber-900 text-amber-300 border-amber-500";

        return (
            <span className={`text-[10px] font-mono px-2 py-1 border ${colorClass}`}>
                {status || "PENDING"}
            </span>
        );
    };

    return (
        <main className="min-h-screen p-8 bg-[#050505] text-gray-200 uppercase tracking-wide">
            <header className="mb-12 border-b-2 border-gray-800 pb-4">
                <h1 className="text-4xl font-extrabold text-white tracking-widest">PMIC // CONTROL CENTER</h1>
                <p className="text-sm font-mono text-gray-500 mt-2">SISTEMA DISTRIBUIDO DE PROCESAMIENTO MULTI-ETAPA</p>
            </header>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">

                {/* COLUMNA IZQUIERDA: COMANDOS Y CONSULTAS */}
                <section className="col-span-1 space-y-8">

                    {/* MODULO 1: LAUNCH COMMAND (ACCION 1) */}
                    <div className="border-2 border-gray-800 bg-[#0a0a0a] p-6 shadow-2xl relative overflow-hidden">
                        <div className="absolute top-0 left-0 w-1 h-full bg-cyan-600"></div>
                        <h2 className="text-sm font-bold border-b border-gray-800 pb-2 mb-6 tracking-widest text-cyan-500">
                            MODULO: NUEVO PROCESAMIENTO
                        </h2>

                        <div className="space-y-6">
                            <div>
                                <label className="block text-[10px] font-mono text-gray-500 mb-2">URLS DE ENTRADA</label>
                                <textarea
                                    className="w-full h-24 bg-black border border-gray-800 text-gray-300 font-mono text-xs p-3 focus:border-cyan-500 focus:outline-none transition-colors rounded-none resize-none"
                                    placeholder="https://servidor.com/img1.jpg"
                                    value={urlsInput}
                                    onChange={(e) => setUrlsInput(e.target.value)}
                                />
                            </div>

                            <div className="grid grid-cols-2 gap-2">
                                <div className="border border-gray-800 p-2 bg-[#080808]">
                                    <label className="block text-[9px] font-mono text-gray-600">WORKERS DESCARGA</label>
                                    <input type="number" min="1" className="w-full bg-transparent text-sm font-mono text-white mt-1 focus:outline-none" value={workersDownload} onChange={(e) => setWorkersDownload(Number(e.target.value))} />
                                </div>
                                <div className="border border-gray-800 p-2 bg-[#080808]">
                                    <label className="block text-[9px] font-mono text-gray-600">WORKERS ESCALADO</label>
                                    <input type="number" min="1" className="w-full bg-transparent text-sm font-mono text-white mt-1 focus:outline-none" value={workersResize} onChange={(e) => setWorkersResize(Number(e.target.value))} />
                                </div>
                                <div className="border border-gray-800 p-2 bg-[#080808]">
                                    <label className="block text-[9px] font-mono text-gray-600">WORKERS COMPRESION</label>
                                    <input type="number" min="1" className="w-full bg-transparent text-sm font-mono text-white mt-1 focus:outline-none" value={workersConvert} onChange={(e) => setWorkersConvert(Number(e.target.value))} />
                                </div>
                                <div className="border border-gray-800 p-2 bg-[#080808]">
                                    <label className="block text-[9px] font-mono text-gray-600">WORKERS MARCA AGUA</label>
                                    <input type="number" min="1" className="w-full bg-transparent text-sm font-mono text-white mt-1 focus:outline-none" value={workersWatermark} onChange={(e) => setWorkersWatermark(Number(e.target.value))} />
                                </div>
                            </div>

                            {errorV && <div className="p-2 border-l-2 border-red-500 bg-red-900/10 text-red-500 font-mono text-[10px]">{errorV}</div>}

                            <button
                                onClick={submitPipeline}
                                className="w-full border-2 border-cyan-800 hover:bg-cyan-900/30 text-cyan-400 font-mono font-bold py-3 text-xs transition-all rounded-none"
                            >
                                EJECUTAR PIPELINE COMMAND
                            </button>
                        </div>
                    </div>

                    {/* MODULO 2: QUERY DIRECTO (ACCION 2) */}
                    <div className="border-2 border-gray-800 bg-[#0a0a0a] p-6 shadow-2xl relative overflow-hidden">
                        <div className="absolute top-0 left-0 w-1 h-full bg-amber-600"></div>
                        <h2 className="text-sm font-bold border-b border-gray-800 pb-2 mb-6 tracking-widest text-amber-500">
                            MODULO: CONSULTA HISTORICA
                        </h2>

                        <div className="space-y-4">
                            <div>
                                <label className="block text-[10px] font-mono text-gray-500 mb-2">IDENTIFICADOR DE TAREA (UUID)</label>
                                <input
                                    type="text"
                                    className="w-full bg-black border border-gray-800 text-amber-500 font-mono text-xs p-3 focus:border-amber-500 focus:outline-none transition-colors rounded-none"
                                    placeholder="ej. 550e8400-e29b..."
                                    value={searchId}
                                    onChange={(e) => setSearchId(e.target.value)}
                                />
                            </div>

                            {searchError && <div className="p-2 border-l-2 border-red-500 bg-red-900/10 text-red-500 font-mono text-[10px]">{searchError}</div>}

                            <button
                                onClick={searchExistingJob}
                                className="w-full border-2 border-amber-800 hover:bg-amber-900/30 text-amber-500 font-mono font-bold py-3 text-xs transition-all rounded-none"
                            >
                                CONSULTAR TELEMETRIA QUERY
                            </button>
                        </div>
                    </div>

                </section>

                {/* COLUMNA DERECHA: DASHBOARD DE TELEMETRIA */}
                <section className="col-span-1 lg:col-span-2">
                    {!jobData && !jobId ? (
                        <div className="h-full border-2 border-dashed border-gray-800 flex flex-col items-center justify-center p-12 text-gray-600 font-mono text-xs space-y-4">
                            <div className="animate-pulse">_ SENSOR DE TELEMETRIA OFFLINE _</div>
                            <p>INICIE UNA TAREA O CONSULTE UN UUID EXISTENTE</p>
                        </div>
                    ) : (
                        <div className="space-y-6 animate-fade-in">

                            <div className="border-2 border-gray-800 bg-[#0a0a0a] p-6 flex justify-between items-center relative overflow-hidden">
                                <div className="absolute top-0 right-0 p-2 opacity-10">
                                    <svg width="40" height="40" viewBox="0 0 24 24" fill="white"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-6h2v6zm0-8h-2V7h2v2z" /></svg>
                                </div>
                                <div>
                                    <h3 className="text-[10px] font-mono text-gray-500">ID TAREA ACTIVA</h3>
                                    <div className="text-sm md:text-base font-mono text-white mt-1">{jobId}</div>
                                </div>
                                <div className="text-right z-10">
                                    <h3 className="text-[10px] font-mono text-gray-500 mb-2">ESTADO GLOBAL</h3>
                                    {renderStatusBox(jobData?.job_info?.status || "BUSCANDO DATOS...")}
                                </div>
                            </div>

                            {jobData && (
                                <>
                                    {/* Métricas de tiempo del job */}
                                    <div className="grid grid-cols-2 md:grid-cols-4 gap-4 font-mono">
                                        <div className="border border-gray-800 bg-[#080808] p-4 text-center">
                                            <div className="text-[10px] text-gray-600">INICIO</div>
                                            <div className="text-sm mt-2 text-white">{formatDateTime(jobData.job_info.start_time)}</div>
                                        </div>
                                        <div className="border border-gray-800 bg-[#080808] p-4 text-center">
                                            <div className="text-[10px] text-gray-600">FINALIZACION</div>
                                            <div className="text-sm mt-2 text-white">{formatDateTime(jobData.job_info.end_time)}</div>
                                        </div>
                                        <div className="border border-cyan-900/30 bg-[#080808] p-4 text-center">
                                            <div className="text-[10px] text-cyan-700">TIEMPO TOTAL</div>
                                            <div className="text-xl mt-2 text-cyan-500">{formatDuration(jobData.global_metrics.total_execution_time_sec)}</div>
                                        </div>
                                        <div className="border border-purple-900/30 bg-[#080808] p-4 text-center">
                                            <div className="text-[10px] text-purple-700">TIEMPO PROM/ARCH</div>
                                            <div className="text-xl mt-2 text-purple-500">
                                                {jobData.job_info.total_files > 0
                                                    ? formatDuration(jobData.global_metrics.total_execution_time_sec / jobData.job_info.total_files)
                                                    : "0s"}
                                            </div>
                                        </div>
                                    </div>

                                    {/* Métricas de archivos con porcentajes */}
                                    <div className="grid grid-cols-3 gap-4 font-mono">
                                        <div className="border border-gray-800 bg-[#080808] p-4 text-center">
                                            <div className="text-[10px] text-gray-600">ARCHIVOS TOTALES</div>
                                            <div className="text-2xl mt-2 text-white">{jobData.job_info.total_files}</div>
                                        </div>
                                        <div className="border border-emerald-900/30 bg-[#080808] p-4 text-center">
                                            <div className="text-[10px] text-emerald-700">EXITOSOS</div>
                                            <div className="text-2xl mt-2 text-emerald-500">{jobData.job_info.processed_files}</div>
                                            <div className="text-[10px] text-emerald-600 mt-1">{jobData.global_metrics.success_percentage.toFixed(1)}%</div>
                                        </div>
                                        <div className="border border-red-900/30 bg-[#080808] p-4 text-center">
                                            <div className="text-[10px] text-red-700">FALLIDOS</div>
                                            <div className="text-2xl mt-2 text-red-500">{jobData.job_info.failed_files}</div>
                                            <div className="text-[10px] text-red-600 mt-1">{jobData.global_metrics.failure_percentage.toFixed(1)}%</div>
                                        </div>
                                    </div>

                                    {/* Métricas por etapas */}
                                    <div className="border-2 border-gray-800 bg-[#0a0a0a] p-4">
                                        <div className="border-b border-gray-800 pb-2 mb-4">
                                            <h3 className="text-[10px] font-mono text-gray-500 tracking-widest">METRICAS POR ETAPA</h3>
                                        </div>
                                        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                                            {/* Etapa 1: Descarga */}
                                            <div className="border border-gray-800 bg-[#060606] p-3">
                                                <div className="text-[10px] text-gray-500 mb-2">01 DESCARGA</div>
                                                <div className="space-y-1">
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">PROCESADOS:</span>
                                                        <span className="text-white">{jobData.stage_metrics?.download?.total_processed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">FALLIDOS:</span>
                                                        <span className="text-red-500">{jobData.stage_metrics?.download?.total_failed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO TOTAL:</span>
                                                        <span className="text-cyan-500">{(jobData.stage_metrics?.download?.total_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO PROM:</span>
                                                        <span className="text-amber-500">{(jobData.stage_metrics?.download?.avg_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                </div>
                                            </div>

                                            {/* Etapa 2: Resize */}
                                            <div className="border border-gray-800 bg-[#060606] p-3">
                                                <div className="text-[10px] text-gray-500 mb-2">02 ESCALADO</div>
                                                <div className="space-y-1">
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">PROCESADOS:</span>
                                                        <span className="text-white">{jobData.stage_metrics?.resize?.total_processed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">FALLIDOS:</span>
                                                        <span className="text-red-500">{jobData.stage_metrics?.resize?.total_failed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO TOTAL:</span>
                                                        <span className="text-cyan-500">{(jobData.stage_metrics?.resize?.total_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO PROM:</span>
                                                        <span className="text-amber-500">{(jobData.stage_metrics?.resize?.avg_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                </div>
                                            </div>

                                            {/* Etapa 3: Convert */}
                                            <div className="border border-gray-800 bg-[#060606] p-3">
                                                <div className="text-[10px] text-gray-500 mb-2">03 COMPRESION PNG</div>
                                                <div className="space-y-1">
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">PROCESADOS:</span>
                                                        <span className="text-white">{jobData.stage_metrics?.convert?.total_processed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">FALLIDOS:</span>
                                                        <span className="text-red-500">{jobData.stage_metrics?.convert?.total_failed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO TOTAL:</span>
                                                        <span className="text-cyan-500">{(jobData.stage_metrics?.convert?.total_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO PROM:</span>
                                                        <span className="text-amber-500">{(jobData.stage_metrics?.convert?.avg_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                </div>
                                            </div>

                                            {/* Etapa 4: Watermark */}
                                            <div className="border border-gray-800 bg-[#060606] p-3">
                                                <div className="text-[10px] text-gray-500 mb-2">04 WATERMARK</div>
                                                <div className="space-y-1">
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">PROCESADOS:</span>
                                                        <span className="text-white">{jobData.stage_metrics?.watermark?.total_processed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">FALLIDOS:</span>
                                                        <span className="text-red-500">{jobData.stage_metrics?.watermark?.total_failed || 0}</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO TOTAL:</span>
                                                        <span className="text-cyan-500">{(jobData.stage_metrics?.watermark?.total_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                    <div className="flex justify-between text-[9px]">
                                                        <span className="text-gray-600">TIEMPO PROM:</span>
                                                        <span className="text-amber-500">{(jobData.stage_metrics?.watermark?.avg_time_sec || 0).toFixed(2)}s</span>
                                                    </div>
                                                </div>
                                            </div>
                                        </div>
                                    </div>

                                    <div className="border-2 border-gray-800 bg-black">
                                        <div className="border-b border-gray-800 p-3 text-[10px] text-gray-500 font-mono tracking-widest flex justify-between">
                                            <span>FLUJO DE TRABAJO A NIVEL DE ARCHIVO</span>
                                            <span className="text-cyan-600">AUTO-REFRESH: ON</span>
                                        </div>
                                        <div className="p-4 space-y-4 max-h-[600px] overflow-y-auto custom-scrollbar">
                                            {jobData.job_info.images.map((img) => (
                                                <div key={img.image_id} className="border border-gray-800 bg-[#060606] p-4">
                                                    <div className="text-[10px] font-mono text-gray-400 mb-4 truncate border-b border-gray-800 pb-2">
                                                        [SRC] {img.original_url}
                                                    </div>

                                                    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                                                        <div className="flex flex-col space-y-2">
                                                            <span className="text-[9px] text-gray-600">01 DESCARGA</span>
                                                            {renderStatusBox(img.download_status)}
                                                            <span className="text-[9px] font-mono text-gray-500 text-right">
                                                                {img.download_time_sec > 0 ? `${img.download_time_sec.toFixed(2)}s` : '-'} | {img.download_worker || 'WAIT'}
                                                            </span>
                                                        </div>
                                                        <div className="flex flex-col space-y-2">
                                                            <span className="text-[9px] text-gray-600">02 ESCALADO</span>
                                                            {renderStatusBox(img.resize_status)}
                                                            <span className="text-[9px] font-mono text-gray-500 text-right">
                                                                {img.resize_time_sec > 0 ? `${img.resize_time_sec.toFixed(2)}s` : '-'} | {img.resize_worker || 'WAIT'}
                                                            </span>
                                                        </div>
                                                        <div className="flex flex-col space-y-2">
                                                            <span className="text-[9px] text-gray-600">03 COMPRESION PNG</span>
                                                            {renderStatusBox(img.convert_status)}
                                                            <span className="text-[9px] font-mono text-gray-500 text-right">
                                                                {img.convert_time_sec > 0 ? `${img.convert_time_sec.toFixed(2)}s` : '-'} | {img.convert_worker || 'WAIT'}
                                                            </span>
                                                        </div>
                                                        <div className="flex flex-col space-y-2">
                                                            <span className="text-[9px] text-gray-600">04 WATERMARK</span>
                                                            {renderStatusBox(img.watermark_status)}
                                                            <span className="text-[9px] font-mono text-gray-500 text-right">
                                                                {img.watermark_time_sec > 0 ? `${img.watermark_time_sec.toFixed(2)}s` : '-'} | {img.watermark_worker || 'WAIT'}
                                                            </span>
                                                        </div>
                                                    </div>
                                                </div>
                                            ))}
                                        </div>
                                    </div>
                                </>
                            )}
                        </div>
                    )}
                </section>

            </div>

            {/* Estilos CSS adicionales embebidos para dar toques finales */}
            <style dangerouslysetcontent={{
                __html: `
        .custom-scrollbar::-webkit-scrollbar { width: 6px; }
        .custom-scrollbar::-webkit-scrollbar-track { background: #000; border-left: 1px solid #111; }
        .custom-scrollbar::-webkit-scrollbar-thumb { background: #333; }
        .custom-scrollbar::-webkit-scrollbar-thumb:hover { background: #555; }
        .animate-fade-in { animation: fadeIn 0.3s ease-in-out; }
        @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }
      `}} />
        </main>
    );
}
