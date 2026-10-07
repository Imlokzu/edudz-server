import Foundation
import PDFKit
import AppKit

// Rasterize a limited page window for the assistant's image-reading tool.
let args = CommandLine.arguments
if args.count != 4 { exit(2) }
guard let document = PDFDocument(url: URL(fileURLWithPath: args[1])), let start = Int(args[3]), start >= 1 else { exit(3) }
let folder = args[2]
try FileManager.default.createDirectory(atPath: folder, withIntermediateDirectories: true)
for index in (start - 1)..<min(document.pageCount, start + 3) {
    guard let page = document.page(at: index) else { continue }
    let image = page.thumbnail(of: NSSize(width: 1000, height: 1400), for: .mediaBox)
    guard let data = image.tiffRepresentation, let bitmap = NSBitmapImageRep(data: data), let png = bitmap.representation(using: .png, properties: [:]) else { continue }
    try png.write(to: URL(fileURLWithPath: folder + "/page-\(index + 1).png"))
}
