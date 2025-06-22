import { writable, derived } from 'svelte/store';

interface User {
	id: string;
	username: string;
	email: string;
	is_admin: boolean;
	active: boolean;
}

interface AuthState {
	user: User | null;
	apiKey: string | null;
	loading: boolean;
}

function createAuthStore() {
	const { subscribe, set, update } = writable<AuthState>({
		user: null,
		apiKey: localStorage.getItem('apiKey'),
		loading: false
	});

	return {
		subscribe,
		login: async (username: string, password: string) => {
			update(s => ({ ...s, loading: true }));
			try {
				const res = await fetch('/api/v1/auth/login', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ username, password })
				});
				
				if (!res.ok) throw new Error('Login failed');
				
				const data = await res.json();
				localStorage.setItem('apiKey', data.api_key);
			
				set({
					user: data.user,
					apiKey: data.api_key,
					loading: false
				});
				
				return data;
			} catch (error) {
				update(s => ({ ...s, loading: false }));
				throw error;
			}
		},
		logout: () => {
			localStorage.removeItem('apiKey');
			set({ user: null, apiKey: null, loading: false });
		},
		whoami: async () => {
			const apiKey = localStorage.getItem('apiKey');
			if (!apiKey) return;
			
			update(s => ({ ...s, loading: true }));
			try {
				const res = await fetch('/api/v1/auth/whoami', {
					headers: { 'X-API-Key': apiKey }
				});
				
				if (!res.ok) {
					throw new Error('Invalid session');
				}
				
				const user = await res.json();
				update(s => ({ ...s, user, loading: false }));
			} catch (error) {
				localStorage.removeItem('apiKey');
				set({ user: null, apiKey: null, loading: false });
			}
		}
	};
}

export const auth = createAuthStore();
export const isAuthenticated = derived(auth, $auth => !!$auth.apiKey);
export const isAdmin = derived(auth, $auth => $auth.user?.is_admin || false);