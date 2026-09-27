import type { CodegenConfig } from '@graphql-codegen/cli';

// Types for every operation are generated from the Go server's schema, so a
// breaking API change fails `pnpm typecheck` instead of failing in the browser.
const config: CodegenConfig = {
  schema: '../internal/adapter/graphql/schema.graphqls',
  documents: ['src/**/*.{ts,tsx}', '!src/gql/**/*'],
  ignoreNoDocuments: true,
  generates: {
    './src/gql/': {
      preset: 'client',
      presetConfig: { fragmentMasking: false },
      config: {
        useTypeImports: true,
        enumsAsTypes: true,
        scalars: { Time: 'string', JSON: 'unknown', USD: 'string' },
      },
    },
  },
};

export default config;
