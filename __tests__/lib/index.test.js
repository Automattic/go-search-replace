const thisPackage = require( '../../' );
const { replace, validate } = thisPackage;

const { Readable, Writable } = require( 'stream' );
const readableStream = new Readable( {
	read() {
		return true;
	},
} );
const writeableStream = new Writable();

describe( 'go-search-replace', () => {
	describe( 'validate()', () => {
		it( 'fails if a readable stream is not passed as the first argument', () => {
			expect( validate( writeableStream, [ 'thing' ] ) ).toBe( false );
		} );

		it( 'fails if an array is not passed as the second argument', () => {
			expect( validate( readableStream, 'replace-string' ) ).toBe( false );
			expect( validate( readableStream, 234 ) ).toBe( false );
			expect( validate( readableStream, new Set( [ 'thing' ] ) ) ).toBe( false );
		} );

		it( 'passes if a readable stream is passed as the first argument', () => {
			expect( validate( readableStream, [ 'thing' ] ) ).toBe( true );
		} );
	} );
	describe( 'replace()', () => {
		it( 'returns an instance of readable stream', () => {
			expect( replace( readableStream, [ 'thing' ] ) ).toBeInstanceOf( Readable );
		} );
	} );
} );
