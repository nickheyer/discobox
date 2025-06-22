export interface User {
	id: string;
	username: string;
	email: string;
	is_admin: boolean;
	active: boolean;
}

export interface Service {
	id: string;
	name: string;
	endpoints: string[];
	health_path: string;
	weight: number;
	max_conns?: number;
	timeout: string;
	metadata?: Record<string, any>;
	tls?: any;
	strip_prefix?: boolean;
	active: boolean;
	created_at: string;
	updated_at: string;
}

export interface Route {
	id: string;
	priority: number;
	host?: string;
	path_prefix?: string;
	path_regex?: string;
	headers?: Record<string, string>;
	service_id: string;
	middlewares?: string[];
	rewrite_rules?: any[];
	metadata?: Record<string, any>;
}

export interface Metrics {
	uptime: string;
	requests: {
		total: number;
		per_second: number;
		errors: number;
		avg_latency_ms: number;
		p50_latency_ms: number;
		p95_latency_ms: number;
		p99_latency_ms: number;
		error_rate: number;
	};
	system: {
		goroutines: number;
		memory_mb: number;
		cpu_percent: number;
		connections: number;
	};
	services: Record<string, ServiceMetrics>;
}

export interface ServiceMetrics {
	requests: number;
	errors: number;
	avg_latency_ms: number;
	health_status: 'healthy' | 'degraded' | 'unhealthy' | 'unknown';
}

export interface Health {
	status: string;
	timestamp: string;
	version: string;
	build: {
		git_commit: string;
		build_time: string;
		go_version: string;
		platform: string;
	};
	runtime: {
		goroutines: number;
		gomaxprocs: number;
		version: string;
		uptime: string;
		memory_mb: number;
		gc_count: number;
	};
}