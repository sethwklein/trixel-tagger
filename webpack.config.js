module.exports = {
    entry: "./index.jsx",
    output: {
        path: __dirname,
        filename: "build.js",
    },
    module: {
        loaders: [{
            loader: "babel",
            test: /\.jsx$/,
            exclude: /node_modules/,
            query: {
                presets: ["babel-preset-react"],
            },
        }],
    },
};
