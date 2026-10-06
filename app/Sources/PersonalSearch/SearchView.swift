import SwiftUI

struct SearchView: View {
    @State private var query = ""

    var body: some View {
        VStack(spacing: 0) {
            TextField("Search your documents", text: $query)
                .textFieldStyle(.plain)
                .font(.title2)
                .padding(24)
        }
        .frame(minWidth: 560)
    }
}

struct SearchViewPreviews: PreviewProvider {
    static var previews: some View {
        SearchView()
            .frame(width: 680, height: 104)
    }
}
