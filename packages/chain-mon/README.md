# @eth-blast/chain-mon

[![codecov](https://codecov.io/gh/BLASTchain/blast/branch/develop/graph/badge.svg?token=0VTG7PG7YR&flag=chain-mon-tests)](https://codecov.io/gh/BLASTchain/blast)

`chain-mon` is a collection of chain monitoring services.

## Installation

Clone, install, and build the Optimism monorepo:

```
git clone https://github.com/BLASTchain/blast.git
pnpm install
pnpm build
```

## Running a service

Copy `.env.example` into a new file named `.env`, then set the environment variables listed there depending on the service you want to run.
Once your environment variables have been set, run via:

```
pnpm start:<service name>
```

For example, to run `drippie-mon`, execute:

```
pnpm start:drippie-mon
```
