import { writable } from 'svelte/store';

const themes = [
	'light', 'dark', 'cupcake', 'bumblebee', 'emerald', 'corporate',
	'synthwave', 'retro', 'cyberpunk', 'valentine', 'halloween', 'garden',
	'forest', 'aqua', 'lofi', 'pastel', 'fantasy', 'wireframe', 'black',
	'luxury', 'dracula', 'cmyk', 'autumn', 'business', 'acid', 'lemonade',
	'night', 'coffee', 'winter', 'dim', 'nord', 'sunset'
] as const;

type Theme = typeof themes[number];

function createThemeStore() {
	const stored = localStorage.getItem('theme') as Theme;
	const { subscribe, set } = writable<Theme>(stored || 'dark');
	
	return {
		subscribe,
		set: (theme: Theme) => {
			localStorage.setItem('theme', theme);
			document.documentElement.setAttribute('data-theme', theme);
			set(theme);
		},
		themes
	};
}

export const theme = createThemeStore();