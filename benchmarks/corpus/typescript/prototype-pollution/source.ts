interface Config {
  [key: string]: any;
}

function deepMerge(target: Config, source: Config): Config {
  for (const key in source) {
    if (typeof source[key] === 'object' && source[key] !== null) {
      if (!target[key]) {
        target[key] = {};
      }
      deepMerge(target[key], source[key]);
    } else {
      target[key] = source[key];
    }
  }
  return target;
}

// API endpoint that merges user-supplied JSON into config
function updateConfig(userInput: string): Config {
  const defaults: Config = { theme: 'light', lang: 'en' };
  const parsed = JSON.parse(userInput);
  return deepMerge(defaults, parsed);
}
