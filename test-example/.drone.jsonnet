[
    {
        kind: "pipeline",
        name: "example-test",
        steps: [
            {
                name: "test",
                image: "alpine",
                commands: [ "echo hello" ],
            },
        ],
    },
]