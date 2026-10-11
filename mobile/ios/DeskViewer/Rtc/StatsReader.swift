// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import DeskCore
import QuartzCore
@preconcurrency import WebRTC

/**
 * Reads the details panel's numbers from libwebrtc's statistics report:
 * the inbound video (frames decoded, bytes, packets, size) and the
 * candidate pair in use. Adapted from go-link's player app
 * (LiveStatsReader).
 */
enum StatsReader {
    static func sample(_ report: RTCStatisticsReport) -> StatsMeter.Sample {
        let all = report.statistics
        func num(_ v: NSObject?) -> Double? { (v as? NSNumber)?.doubleValue }
        func int64(_ v: NSObject?) -> Int64? { num(v).map { Int64($0) } }

        var out = StatsMeter.Sample(atMs: CACurrentMediaTime() * 1000)
        for s in all.values where s.type == "inbound-rtp" && (s.values["kind"] as? String) == "video" {
            let m = s.values
            out.framesDecoded = int64(m["framesDecoded"])
            out.bytesReceived = int64(m["bytesReceived"])
            out.packetsReceived = int64(m["packetsReceived"])
            out.packetsLost = max(0, int64(m["packetsLost"]) ?? 0)
            out.width = num(m["frameWidth"]).map { Int($0) }
            out.height = num(m["frameHeight"]).map { Int($0) }
        }
        // The pair in use: the transport's selected one, or the nominated pair that succeeded.
        let selectedId = all.values.first { $0.type == "transport" }.flatMap { $0.values["selectedCandidatePairId"] as? String }
        let pair = selectedId.flatMap { all[$0] } ?? all.values.first {
            $0.type == "candidate-pair" && ($0.values["nominated"] as? NSNumber)?.boolValue == true && ($0.values["state"] as? String) == "succeeded"
        }
        if let pair {
            out.rttSeconds = num(pair.values["currentRoundTripTime"])
            out.localType = (pair.values["localCandidateId"] as? String).flatMap { all[$0]?.values["candidateType"] as? String }
            out.remoteType = (pair.values["remoteCandidateId"] as? String).flatMap { all[$0]?.values["candidateType"] as? String }
        }
        return out
    }
}
