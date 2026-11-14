import ExpoModulesCore
import UIKit

private let foldChangeEvent = "foldChange"

/// Reports iPhone Duo's fold to JS in the same shape as the Android module:
/// the active division region of the key window (iOS 27.1+), in points
/// relative to the window. The region is only active while the device is
/// partially folded, so a flat or closed device reports no fold.
///
/// UIKit can move a division region without a layout pass, so the fold is
/// re-read whenever the hinge changes (a `UIHingeInteraction` on the key
/// window) and when the app becomes active.
public class FoldInfoModule: Module {
  private let lock = NSLock()
  private var current: [String: Any]?
  private var interaction: UIInteraction?

  public func definition() -> ModuleDefinition {
    Name("FoldInfo")

    Events(foldChangeEvent)

    // Answered from the cache: UIKit may only be read on the main thread.
    Function("getFold") { () -> [String: Any]? in
      return lock.withLock { current }
    }

    OnCreate {
      Task { @MainActor in
        self.attach()
      }
    }

    OnAppBecomesActive {
      Task { @MainActor in
        self.attach()
      }
    }

    OnDestroy {
      Task { @MainActor in
        if let interaction = self.interaction {
          interaction.view?.removeInteraction(interaction)
        }
        self.interaction = nil
      }
    }
  }

  @MainActor
  private func attach() {
    #if canImport(UIKit, _version: 9127.0.85) // iOS 27.1 SDK
    guard #available(iOS 27.1, *), let window = Utilities.keyWindow() else {
      return
    }
    if interaction?.view !== window {
      if let interaction {
        interaction.view?.removeInteraction(interaction)
      }
      let hinge = UIHingeInteraction { [weak self] _, _ in
        // The regions settle after the hinge update; read now and once more
        // after the next layout.
        self?.refresh()
        DispatchQueue.main.async { self?.refresh() }
      }
      window.addInteraction(hinge)
      interaction = hinge
    }
    #endif
    refresh()
  }

  @MainActor
  private func refresh() {
    let fold = Self.readFold()
    let changed: Bool = lock.withLock {
      let same: Bool
      switch (current, fold) {
      case (nil, nil): same = true
      case let (a?, b?): same = NSDictionary(dictionary: a).isEqual(to: b)
      default: same = false
      }
      if same {
        return false
      }
      current = fold
      return true
    }
    if changed {
      sendEvent(foldChangeEvent, ["fold": fold as Any])
    }
  }

  @MainActor
  private static func readFold() -> [String: Any]? {
    #if canImport(UIKit, _version: 9127.0.85) // iOS 27.1 SDK
    guard #available(iOS 27.1, *), let window = Utilities.keyWindow() else {
      return nil
    }
    guard let region = window.reservedRegions(kind: .division).first(where: { $0.isActive }) else {
      return nil
    }
    let f = region.frame
    if f.width <= 0 && f.height <= 0 {
      return nil
    }
    return [
      "left": Double(f.minX),
      "top": Double(f.minY),
      "width": Double(f.width),
      "height": Double(f.height),
      "orientation": f.height >= f.width ? "vertical" : "horizontal",
      // Active only while partially folded.
      "state": "halfOpened",
      "separating": true,
      // The frame already includes the margins to keep clear.
      "occludes": true,
    ]
    #else
    return nil
    #endif
  }
}
