/**
 * External dependencies
 */
const { spawn } = require( 'child_process' );
const stream = require( 'stream' );

/**
 * Internal dependencies
 */

/**
 * Validate the library inputs
 *
 * @param {Readable} streamObj The readable stream
 * @param {Array} replacements An array of replacements
 * @return {Boolean} valid
 */
function validate( streamObj, replacements ) {
	let valid = true;
	if ( ! ( streamObj instanceof stream.Readable ) ) {
		console.error( 'The first argument must be an instance of a Readable stream.' );
		valid = false;
	}
	if ( ! Array.isArray( replacements ) ) {
		console.error( 'The second argument must be a array.' );
		valid = false;
	}
	return valid;
}

/**
 * The Search and Replace Process
 *
 * @param {Readable} streamObj The readable stream
 * @param {Array} replacements An array of replacements
 * @return {Readable} streamObj
 */
function replace( streamObj, replacements ) {
	const valid = validate( streamObj, replacements );
	if ( ! valid ) {
		process.exit( 1 );
	}
	const replaceProcess = spawn( 'go-search-replace', replacements, {
		stdio: [ 'pipe', 'pipe', process.stderr ],
	} );
	replaceProcess.on( 'error', err => {
		console.error( '\n' + err.toString() );
		process.exit( 1 );
	} );

	streamObj.pipe( replaceProcess.stdin );
	streamObj = replaceProcess.stdout;
	return streamObj;
}

module.exports = {
	replace,
	validate,
};
