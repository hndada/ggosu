import { analyze } from './beatfit.js';
self.onmessage = ({ data }) => {
    try { self.postMessage({ result: analyze(data.channels, data.sampleRate) }); }
    catch (error) { self.postMessage({ error: error.message }); }
};
