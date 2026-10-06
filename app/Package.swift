// swift-tools-version: 6.2

import PackageDescription

let package = Package(
    name: "PersonalSearch",
    platforms: [
        .macOS(.v14),
    ],
    products: [
        .executable(
            name: "PersonalSearch",
            targets: ["PersonalSearch"]
        ),
    ],
    targets: [
        .executableTarget(
            name: "PersonalSearch"
        ),
    ]
)

