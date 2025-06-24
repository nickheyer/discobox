import { browser } from '$app/environment';

import type { Service, Route, Metrics, Health } from '$lib/types';

class ApiClient {
	private baseUrl = '/api/v1';
	
	private getHeaders(): HeadersInit {
		const headers: HeadersInit = {
			'Content-Type': 'application/json'
		};
		
		const apiKey = localStorage.getItem('apiKey');
		if (apiKey) {
			headers['X-API-Key'] = apiKey;
		}
		
		return headers;
	}
	
	async request<T>(path: string, options: RequestInit = {}): Promise<T> {
		const res = await fetch(`${this.baseUrl}${path}`, {
			...options,
			headers: {
				...this.getHeaders(),
				...options.headers
			}
		});
		
		if (!res.ok) {
			const error = await res.text();
			throw new Error(error || `HTTP ${res.status}`);
		}
		
		// Handle 204 No Content
		if (res.status === 204) {
			return null as T;
		}
		
		return res.json();
	}
	
	// Services
	async getServices() {
		return this.request<Service[]>('/services');
	}
	
	async getService(id: string) {
		return this.request<Service>(`/services/${id}`);
	}
	
	async createService(service: Partial<Service>) {
		return this.request<Service>('/services', {
			method: 'POST',
			body: JSON.stringify(service)
		});
	}
	
	async updateService(id: string, service: Partial<Service>) {
		return this.request<Service>(`/services/${id}`, {
			method: 'PUT',
			body: JSON.stringify(service)
		});
	}
	
	async deleteService(id: string) {
		return this.request<void>(`/services/${id}`, {
			method: 'DELETE'
		});
	}
	
	// Routes
	async getRoutes() {
		return this.request<Route[]>('/routes');
	}
	
	async getRoute(id: string) {
		return this.request<Route>(`/routes/${id}`);
	}
	
	async createRoute(route: Partial<Route>) {
		return this.request<Route>('/routes', {
			method: 'POST',
			body: JSON.stringify(route)
		});
	}
	
	async updateRoute(id: string, route: Partial<Route>) {
		return this.request<Route>(`/routes/${id}`, {
			method: 'PUT',
			body: JSON.stringify(route)
		});
	}
	
	async deleteRoute(id: string) {
		return this.request<void>(`/routes/${id}`, {
			method: 'DELETE'
		});
	}
	
	// Metrics
	async getMetrics() {
		return this.request<Metrics>('/stats');
	}
	
	// Health
	async getHealth() {
		const res = await fetch('/health');
		return res.json() as Promise<Health>;
	}
}

export const api = new ApiClient();