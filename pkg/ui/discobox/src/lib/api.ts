import { browser } from '$app/environment';

class ApiClient {
	private baseUrl = '/api/v1';
	
	private getHeaders(): HeadersInit {
		const headers: HeadersInit = {
			'Content-Type': 'application/json'
		};
		
		if (browser) {
			const apiKey = localStorage.getItem('apiKey');
			if (apiKey) {
				headers['X-API-Key'] = apiKey;
			}
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
		
		return res.json();
	}
	
	// Services
	async getServices() {
		return this.request<any[]>('/services');
	}
	
	async getService(id: string) {
		return this.request<any>(`/services/${id}`);
	}
	
	async createService(service: any) {
		return this.request('/services', {
			method: 'POST',
			body: JSON.stringify(service)
		});
	}
	
	async updateService(id: string, service: any) {
		return this.request(`/services/${id}`, {
			method: 'PUT',
			body: JSON.stringify(service)
		});
	}
	
	async deleteService(id: string) {
		return this.request(`/services/${id}`, {
			method: 'DELETE'
		});
	}
	
	// Routes
	async getRoutes() {
		return this.request<any[]>('/routes');
	}
	
	async getRoute(id: string) {
		return this.request<any>(`/routes/${id}`);
	}
	
	async createRoute(route: any) {
		return this.request('/routes', {
			method: 'POST',
			body: JSON.stringify(route)
		});
	}
	
	async updateRoute(id: string, route: any) {
		return this.request(`/routes/${id}`, {
			method: 'PUT',
			body: JSON.stringify(route)
		});
	}
	
	async deleteRoute(id: string) {
		return this.request(`/routes/${id}`, {
			method: 'DELETE'
		});
	}
	
	// Metrics
	async getMetrics() {
		return this.request<any>('/metrics');
	}
	
	// Health
	async getHealth() {
		const res = await fetch('/health');
		return res.json();
	}
}

export const api = new ApiClient();