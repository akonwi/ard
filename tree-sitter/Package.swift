// swift-tools-version:5.3
import PackageDescription

let package = Package(
    name: "TreeSitterArd",
    products: [
        .library(name: "TreeSitterArd", targets: ["TreeSitterArd"]),
    ],
    dependencies: [
        .package(url: "https://github.com/ChimeHQ/SwiftTreeSitter", from: "0.8.0"),
    ],
    targets: [
        .target(
            name: "TreeSitterArd",
            dependencies: [],
            path: ".",
            sources: [
                "src/parser.c",
                // NOTE: if your language has an external scanner, add it here.
            ],
            resources: [
                .copy("queries")
            ],
            publicHeadersPath: "bindings/swift",
            cSettings: [.headerSearchPath("src")]
        ),
        .testTarget(
            name: "TreeSitterArdTests",
            dependencies: [
                "SwiftTreeSitter",
                "TreeSitterArd",
            ],
            path: "bindings/swift/TreeSitterArdTests"
        )
    ],
    cLanguageStandard: .c11
)
